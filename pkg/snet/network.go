package snet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/SkycoinProject/dmsg/netutil"

	"github.com/SkycoinProject/dmsg"
	"github.com/SkycoinProject/dmsg/cipher"
	"github.com/SkycoinProject/dmsg/disc"
	"github.com/SkycoinProject/skycoin/src/util/logging"

	"github.com/SkycoinProject/skywire-mainnet/pkg/app/appevent"
	"github.com/SkycoinProject/skywire-mainnet/pkg/snet/arclient"
	"github.com/SkycoinProject/skywire-mainnet/pkg/snet/stcp"
	"github.com/SkycoinProject/skywire-mainnet/pkg/snet/stcph"
	"github.com/SkycoinProject/skywire-mainnet/pkg/snet/stcpr"
	"github.com/SkycoinProject/skywire-mainnet/pkg/snet/sudp"
)

var log = logging.MustGetLogger("snet")

// Default ports.
// TODO(evanlinjin): Define these properly. These are currently random.
const (
	SetupPort      = uint16(36)  // Listening port of a setup node.
	AwaitSetupPort = uint16(136) // Listening port of a visor for setup operations.
	TransportPort  = uint16(45)  // Listening port of a visor for incoming transports.
)

var (
	// ErrUnknownNetwork occurs on attempt to dial an unknown network type.
	ErrUnknownNetwork = errors.New("unknown network type")
	knownNetworks     = map[string]struct{}{
		dmsg.Type:  {},
		stcp.Type:  {},
		stcpr.Type: {},
		stcph.Type: {},
		sudp.Type:  {},
	}
)

// TODO (darkren) move this below
func IsKnownNetwork(netType string) bool {
	_, ok := knownNetworks[netType]
	return ok
}

// NetworkConfig is a common interface for network configs.
type NetworkConfig interface {
	Type() string
}

// DmsgConfig defines config for Dmsg network.
type DmsgConfig struct {
	Discovery     string `json:"discovery"`
	SessionsCount int    `json:"sessions_count"`
}

// Type returns DmsgType.
func (c *DmsgConfig) Type() string {
	return dmsg.Type
}

// STCPConfig defines config for STCP network.
type STCPConfig struct {
	PKTable   map[cipher.PubKey]string `json:"pk_table"`
	LocalAddr string                   `json:"local_address"`
}

// Type returns STCPType.
func (c *STCPConfig) Type() string {
	return stcp.Type
}

// STCPRConfig defines config for STCPR network.
type STCPRConfig struct {
	AddressResolver string `json:"address_resolver"`
	LocalAddr       string `json:"local_address"`
}

// Type returns STCPRType.
func (c *STCPRConfig) Type() string {
	return stcpr.Type
}

// STCPHConfig defines config for STCPH network.
type STCPHConfig struct {
	AddressResolver string `json:"address_resolver"`
}

// Type returns STCPHType.
func (c *STCPHConfig) Type() string {
	return stcph.Type
}

// SUDPConfig defines config for SUDP network.
type SUDPConfig struct {
	PKTable   map[cipher.PubKey]string `json:"pk_table"`
	LocalAddr string                   `json:"local_address"`
}

// Type returns STCPType.
func (c *SUDPConfig) Type() string {
	return sudp.Type
}

// Config represents a network configuration.
type Config struct {
	PubKey         cipher.PubKey
	SecKey         cipher.SecKey
	NetworkConfigs NetworkConfigs
}

// NetworkConfigs represents all network configs.
type NetworkConfigs struct {
	Dmsg  *DmsgConfig  // The dmsg service will not be started if nil.
	STCP  *STCPConfig  // The stcp service will not be started if nil.
	STCPR *STCPRConfig // The stcpr service will not be started if nil.
	STCPH *STCPHConfig // The stcph service will not be started if nil.
	SUDP  *SUDPConfig  // The sudp service will not be started if nil.
}

// NetworkClients represents all network clients.
type NetworkClients struct {
	DmsgC *dmsg.Client
	StcpC *stcp.Client
	SudpC *sudp.Client

	stcprCMu      sync.RWMutex
	stcprCReadyCh chan struct{}
	stcprC        *stcpr.Client

	stcphCMu      sync.RWMutex
	stcphCReadyCh chan struct{}
	stcphC        *stcph.Client
}

func (nc *NetworkClients) StcprC() *stcpr.Client {
	nc.stcprCMu.RLock()
	c := nc.stcprC
	nc.stcprCMu.RUnlock()
	return c
}

func (nc *NetworkClients) StcphC() *stcph.Client {
	nc.stcphCMu.RLock()
	c := nc.stcphC
	nc.stcphCMu.RUnlock()
	return c
}

// Network represents a network between nodes in Skywire.
type Network struct {
	conf       Config
	networksMu sync.RWMutex
	networks   map[string]struct{} // networks to be used with transports
	clients    *NetworkClients

	onNewNetworkTypeMu sync.Mutex
	onNewNetworkType   func(netType string)
}

// TODO (darkren): move it below
func (n *Network) OnNewNetworkType(callback func(netType string)) {
	n.onNewNetworkTypeMu.Lock()
	n.onNewNetworkType = callback
	n.onNewNetworkTypeMu.Unlock()
}

func (n *Network) IsNetworkReady(netType string) bool {
	n.networksMu.Lock()
	_, ok := n.networks[netType]
	n.networksMu.Unlock()
	return ok
}

// New creates a network from a config.
func New(conf Config, eb *appevent.Broadcaster) (*Network, error) {
	clients := NetworkClients{
		stcprCReadyCh: make(chan struct{}),
		stcphCReadyCh: make(chan struct{}),
	}

	if conf.NetworkConfigs.Dmsg != nil {
		dmsgConf := &dmsg.Config{
			MinSessions: conf.NetworkConfigs.Dmsg.SessionsCount,
			Callbacks: &dmsg.ClientCallbacks{
				OnSessionDial: func(network, addr string) error {
					data := appevent.TCPDialData{RemoteNet: network, RemoteAddr: addr}
					event := appevent.NewEvent(appevent.TCPDial, data)
					_ = eb.Broadcast(context.Background(), event) //nolint:errcheck
					// @evanlinjin: An error is not returned here as this will cancel the session dial.
					return nil
				},
				OnSessionDisconnect: func(network, addr string, _ error) {
					data := appevent.TCPCloseData{RemoteNet: network, RemoteAddr: addr}
					event := appevent.NewEvent(appevent.TCPClose, data)
					_ = eb.Broadcast(context.Background(), event) //nolint:errcheck
				},
			},
		}
		clients.DmsgC = dmsg.NewClient(conf.PubKey, conf.SecKey, disc.NewHTTP(conf.NetworkConfigs.Dmsg.Discovery), dmsgConf)
		clients.DmsgC.SetLogger(logging.MustGetLogger("snet.dmsgC"))
	}

	// TODO(nkryuchkov): Generic code for clients below.
	if conf.NetworkConfigs.STCP != nil {
		clients.StcpC = stcp.NewClient(conf.PubKey, conf.SecKey, stcp.NewTable(conf.NetworkConfigs.STCP.PKTable))
		clients.StcpC.SetLogger(logging.MustGetLogger("snet.stcpC"))
	}

	if conf.NetworkConfigs.STCPR != nil || conf.NetworkConfigs.STCPH != nil {
		var addressResolver arclient.APIClient
		var addressResolverMu sync.Mutex

		// this one will be released as soon as address resolver is ready
		addressResolverMu.Lock()
		// goroutine to setup AR
		go func() {
			defer addressResolverMu.Unlock()

			stcprRetrier := netutil.NewRetrier(logging.MustGetLogger("snet.stcpr.retrier"), 1*time.Second, 10*time.Second, 0, 1)

			var ar arclient.APIClient
			err := stcprRetrier.Do(context.Background(), func() error {
				var err error
				ar, err = arclient.NewHTTP(conf.NetworkConfigs.STCPR.AddressResolver, conf.PubKey, conf.SecKey)
				if err != nil {
					return err
				}

				addressResolver = ar

				return nil
			})

			log := logging.MustGetLogger("snet")
			log.WithError(err).Error("failed to connect to address resolver")
		}()

		// setup stcpr
		if conf.NetworkConfigs.STCPR != nil {
			go func() {
				addressResolverMu.Lock()
				ar := addressResolver
				addressResolverMu.Unlock()

				clients.stcprCMu.Lock()
				clients.stcprC = stcpr.NewClient(conf.PubKey, conf.SecKey, ar, conf.NetworkConfigs.STCPR.LocalAddr)
				clients.stcprC.SetLogger(logging.MustGetLogger("snet.stcprC"))
				close(clients.stcprCReadyCh)
				clients.stcprCMu.Unlock()
			}()
		}

		// setup stcph
		if conf.NetworkConfigs.STCPH != nil {
			go func() {
				addressResolverMu.Lock()
				ar := addressResolver
				addressResolverMu.Unlock()

				clients.stcphCMu.Lock()
				clients.stcphC = stcph.NewClient(conf.PubKey, conf.SecKey, ar)
				clients.stcphC.SetLogger(logging.MustGetLogger("snet.stcphC"))
				close(clients.stcphCReadyCh)
				clients.stcphCMu.Unlock()
			}()
		}
	}

	if conf.NetworkConfigs.SUDP != nil {
		clients.SudpC = sudp.NewClient(conf.PubKey, conf.SecKey, stcp.NewTable(conf.NetworkConfigs.SUDP.PKTable))
		clients.SudpC.SetLogger(logging.MustGetLogger("snet.sudpC"))
	}

	return NewRaw(conf, &clients), nil
}

// NewRaw creates a network from a config and a dmsg client.
func NewRaw(conf Config, clients *NetworkClients) *Network {
	n := &Network{
		conf:     conf,
		networks: make(map[string]struct{}),
		clients:  clients,
	}

	if clients.DmsgC != nil {
		n.addNetworkType(dmsg.Type)
	}

	if clients.StcpC != nil {
		n.addNetworkType(stcp.Type)
	}

	if clients.StcprC() != nil {
		n.addNetworkType(stcpr.Type)
	} else {
		go func() {
			<-clients.stcprCReadyCh

			if clients.StcprC() != nil {
				n.addNetworkType(stcpr.Type)
			}
		}()
	}

	if clients.StcphC() != nil {
		n.addNetworkType(stcph.Type)
	} else {
		go func() {
			<-clients.stcphCReadyCh

			if clients.StcphC() != nil {
				n.addNetworkType(stcph.Type)
			}
		}()
	}

	if clients.SudpC != nil {
		n.addNetworkType(sudp.Type)
	}

	return n
}

// Init initiates server connections.
func (n *Network) Init() error {
	if n.clients.DmsgC != nil {
		time.Sleep(200 * time.Millisecond)
		go n.clients.DmsgC.Serve()
		time.Sleep(200 * time.Millisecond)
	}

	if n.conf.NetworkConfigs.STCP != nil {
		if n.clients.StcpC != nil && n.conf.NetworkConfigs.STCP.LocalAddr != "" {
			if err := n.clients.StcpC.Serve(n.conf.NetworkConfigs.STCP.LocalAddr); err != nil {
				return fmt.Errorf("failed to initiate 'stcp': %w", err)
			}
		} else {
			log.Infof("No config found for stcp")
		}
	}

	if n.conf.NetworkConfigs.STCPR != nil {
		go func() {
			<-n.clients.stcprCReadyCh

			stcprC := n.clients.StcprC()
			if stcprC != nil && n.conf.NetworkConfigs.STCPR.LocalAddr != "" {
				if err := stcprC.Serve(); err != nil {
					log.WithError(err).Error("failed to initiate 'stcpr'")
				}
			} else {
				log.Infof("No config found for stcpr")
			}
		}()
	}

	if n.conf.NetworkConfigs.STCPH != nil {
		go func() {
			<-n.clients.stcphCReadyCh

			stcphC := n.clients.StcphC()
			if stcphC != nil {
				if err := stcphC.Serve(); err != nil {
					log.WithError(err).Error("failed to initiate 'stcph'")
				}
			} else {
				log.Infof("No config found for stcph")
			}
		}()
	}

	if n.conf.NetworkConfigs.SUDP != nil {
		if n.clients.SudpC != nil && n.conf.NetworkConfigs.SUDP.LocalAddr != "" {
			if err := n.clients.SudpC.Serve(n.conf.NetworkConfigs.SUDP.LocalAddr); err != nil {
				return fmt.Errorf("failed to initiate 'sudp': %w", err)
			}
		} else {
			log.Infof("No config found for sudp")
		}
	}

	return nil
}

// Close closes underlying connections.
func (n *Network) Close() error {
	n.networksMu.Lock()
	defer n.networksMu.Unlock()

	wg := new(sync.WaitGroup)
	wg.Add(len(n.networks))

	var dmsgErr error
	if n.clients.DmsgC != nil {
		go func() {
			dmsgErr = n.clients.DmsgC.Close()
			wg.Done()
		}()
	}

	var stcpErr error
	if n.clients.StcpC != nil {
		go func() {
			stcpErr = n.clients.StcpC.Close()
			wg.Done()
		}()
	}

	var stcprErr error
	n.clients.stcprCMu.Lock()
	// TODO (darkren): we're maybe currently setting up the client at this point.
	// so in this case, we should stop trying to set it up. or probably just forget about it
	if n.clients.stcprC != nil {
		go func() {
			defer n.clients.stcprCMu.Unlock()

			stcprErr = n.clients.stcprC.Close()
			wg.Done()
		}()
	} else {
		n.clients.stcprCMu.Unlock()
	}

	var stcphErr error
	n.clients.stcphCMu.Lock()
	if n.clients.stcphC != nil {
		go func() {
			defer n.clients.stcphCMu.Unlock()

			stcphErr = n.clients.stcphC.Close()
			wg.Done()
		}()
	} else {
		n.clients.stcphCMu.Unlock()
	}

	var sudpErr error
	if n.clients.SudpC != nil {
		go func() {
			sudpErr = n.clients.SudpC.Close()
			wg.Done()
		}()
	}

	wg.Wait()

	if dmsgErr != nil {
		return dmsgErr
	}

	if stcpErr != nil {
		return stcpErr
	}

	if stcprErr != nil {
		return stcprErr
	}

	if stcphErr != nil {
		return stcphErr
	}

	if sudpErr != nil {
		return sudpErr
	}

	return nil
}

// LocalPK returns local public key.
func (n *Network) LocalPK() cipher.PubKey { return n.conf.PubKey }

// LocalSK returns local secure key.
func (n *Network) LocalSK() cipher.SecKey { return n.conf.SecKey }

// TransportNetworks returns network types that are used for transports.
func (n *Network) TransportNetworks() []string {
	n.networksMu.RLock()
	networks := make([]string, len(n.networks))
	for network := range n.networks {
		log.Infof("APPENDING TO NETWORKS: %s", network)
		networks = append(networks, network)
	}
	n.networksMu.RUnlock()

	return networks
}

// Dmsg returns underlying dmsg client.
func (n *Network) Dmsg() *dmsg.Client { return n.clients.DmsgC }

// STcp returns the underlying stcp.Client.
func (n *Network) STcp() *stcp.Client { return n.clients.StcpC }

// STcpr returns the underlying stcpr.Client.
func (n *Network) STcpr() *stcpr.Client { return n.clients.StcprC() }

// STcpH returns the underlying stcph.Client.
func (n *Network) STcpH() *stcph.Client { return n.clients.StcphC() }

// SUdp returns the underlying sudp.Client.
func (n *Network) SUdp() *sudp.Client { return n.clients.SudpC }

// Dial dials a visor by its public key and returns a connection.
func (n *Network) Dial(ctx context.Context, network string, pk cipher.PubKey, port uint16) (*Conn, error) {
	switch network {
	case dmsg.Type:
		addr := dmsg.Addr{
			PK:   pk,
			Port: port,
		}

		conn, err := n.clients.DmsgC.Dial(ctx, addr)
		if err != nil {
			return nil, fmt.Errorf("dmsg client: %w", err)
		}

		return makeConn(conn, network), nil
	case stcp.Type:
		conn, err := n.clients.StcpC.Dial(ctx, pk, port)
		if err != nil {
			return nil, fmt.Errorf("stcpr client: %w", err)
		}

		return makeConn(conn, network), nil
	case stcpr.Type:
		stcprC := n.clients.StcprC()
		if stcprC == nil {
			return nil, errors.New("stcpr client is not ready")
		}

		conn, err := stcprC.Dial(ctx, pk, port)
		if err != nil {
			return nil, fmt.Errorf("stcpr client: %w", err)
		}

		return makeConn(conn, network), nil
	case stcph.Type:
		stcphC := n.clients.StcphC()
		if stcphC == nil {
			return nil, errors.New("stcph client is not ready")
		}

		conn, err := stcphC.Dial(ctx, pk, port)
		if err != nil {
			return nil, fmt.Errorf("stcph client: %w", err)
		}

		return makeConn(conn, network), nil
	case sudp.Type:
		conn, err := n.clients.SudpC.Dial(ctx, pk, port)
		if err != nil {
			return nil, fmt.Errorf("sudpr client: %w", err)
		}

		return makeConn(conn, network), nil
	default:
		return nil, ErrUnknownNetwork
	}
}

// Listen listens on the specified port.
func (n *Network) Listen(network string, port uint16) (*Listener, error) {
	switch network {
	case dmsg.Type:
		lis, err := n.clients.DmsgC.Listen(port)
		if err != nil {
			return nil, err
		}

		return makeListener(lis, network), nil
	case stcp.Type:
		lis, err := n.clients.StcpC.Listen(port)
		if err != nil {
			return nil, err
		}

		return makeListener(lis, network), nil
	case stcpr.Type:
		stcprC := n.clients.StcprC()
		if stcprC == nil {
			return nil, errors.New("stcpr client is not ready")
		}

		lis, err := stcprC.Listen(port)
		if err != nil {
			return nil, err
		}

		return makeListener(lis, network), nil
	case stcph.Type:
		stcphC := n.clients.StcphC()
		if stcphC == nil {
			return nil, errors.New("stcph client is not ready")
		}

		lis, err := stcphC.Listen(port)
		if err != nil {
			return nil, err
		}

		return makeListener(lis, network), nil
	case sudp.Type:
		lis, err := n.clients.SudpC.Listen(port)
		if err != nil {
			return nil, err
		}

		return makeListener(lis, network), nil
	default:
		return nil, ErrUnknownNetwork
	}
}

func (n *Network) addNetworkType(netType string) {
	n.networksMu.Lock()
	defer n.networksMu.Unlock()

	if _, ok := n.networks[netType]; !ok {
		n.networks[netType] = struct{}{}
		n.onNewNetworkTypeMu.Lock()
		if n.onNewNetworkType != nil {
			n.onNewNetworkType(netType)
		}
		n.onNewNetworkTypeMu.Unlock()
	}
}

func disassembleAddr(addr net.Addr) (pk cipher.PubKey, port uint16) {
	strs := strings.Split(addr.String(), ":")
	if len(strs) != 2 {
		panic(fmt.Errorf("network.disassembleAddr: %v %s", "invalid addr", addr.String()))
	}

	if err := pk.Set(strs[0]); err != nil {
		panic(fmt.Errorf("network.disassembleAddr: %v %s", err, addr.String()))
	}

	if strs[1] != "~" {
		if _, err := fmt.Sscanf(strs[1], "%d", &port); err != nil {
			panic(fmt.Errorf("network.disassembleAddr: %w", err))
		}
	}

	return
}
