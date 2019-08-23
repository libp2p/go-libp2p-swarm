package testing

import (
	"bytes"
	"io"

	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/network"
)

var log = logging.Logger("swarm_test")

func EchoStreamHandler(stream network.Stream) {
	go func() {
		defer stream.Close()

		// pull out the ipfs conn
		c := stream.Conn()
		log.Infof("%s ponging to %s", c.LocalPeer(), c.RemotePeer())

		buf := make([]byte, 4)

		for {
			if _, err := stream.Read(buf); err != nil {
				if err != io.EOF {
					log.Error("ping receive error:", err)
				}
				return
			}

			if !bytes.Equal(buf, []byte("ping")) {
				log.Errorf("ping receive error: ping != %s %v", buf, buf)
				return
			}

			if _, err := stream.Write([]byte("pong")); err != nil {
				log.Error("pond send error:", err)
				return
			}
		}
	}()
}
