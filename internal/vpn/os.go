package vpn

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
)

func parseCIDR(ipCIDR string) (ipStr, netmask string, err error) {
	ip, net, err := net.ParseCIDR(ipCIDR)
	if err != nil {
		return "", "", err
	}

	return ip.String(), fmt.Sprintf("%d.%d.%d.%d", net.Mask[0], net.Mask[1], net.Mask[2], net.Mask[3]), nil
}

//nolint:unparam
func run(bin string, args ...string) error {
	fullCmd := bin + " " + strings.Join(args, " ")

	cmd := exec.Command(bin, args...) //nolint:gosec

	stderrBuf := bytes.NewBuffer(nil)

	cmd.Stderr = io.MultiWriter(os.Stderr, stderrBuf)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return NewErrorWithStderr(fmt.Errorf("error running command \"%s\": %w", fullCmd, err),
			stderrBuf.Bytes())
	}

	return nil
}
