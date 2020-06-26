package directtransport

import (
	"context"

	"github.com/SkycoinProject/dmsg/cipher"
	"github.com/SkycoinProject/skycoin/src/util/logging"
)

// Client is a direct transport client.
type Client interface { // TODO(nkryuchkov): use
	SetLogger(log *logging.Logger) // TODO(nkryuchkov): remove
	Dial(ctx context.Context, rPK cipher.PubKey, rPort uint16) (*Conn, error)
	Listen(lPort uint16) (*Listener, error)
	Serve() error
	Close() error
	Type() string
}
