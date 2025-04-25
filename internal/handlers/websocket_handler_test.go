package handlers

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type wsWriter interface {
	WriteMessage(messageType int, data []byte) error
}

type mockConn struct {
	written [][]byte
	fail    bool
}

func (m *mockConn) WriteMessage(messageType int, data []byte) error {
	if m.fail {
		return errors.New("write failed")
	}
	m.written = append(m.written, data)
	return nil
}

// roundRobinDispatch sends payload to one of the conns using round-robin, returns error if none
func roundRobinDispatch(conns []wsWriter, rrIndex *int, payload []byte) error {
	if len(conns) == 0 {
		return fmt.Errorf("no client connected")
	}
	conn := conns[*rrIndex%len(conns)]
	err := conn.WriteMessage(1, payload)
	*rrIndex = (*rrIndex + 1) % len(conns)
	return err
}

func TestRoundRobinDispatch(t *testing.T) {
	t.Run("single client", func(t *testing.T) {
		mc := &mockConn{}
		rr := 0
		err := roundRobinDispatch([]wsWriter{mc}, &rr, []byte("hello"))
		assert.NoError(t, err)
		assert.Equal(t, [][]byte{[]byte("hello")}, mc.written)
	})

	t.Run("no client connected", func(t *testing.T) {
		rr := 0
		err := roundRobinDispatch([]wsWriter{}, &rr, []byte("x"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no client connected")
	})

	t.Run("round robin", func(t *testing.T) {
		mc1 := &mockConn{}
		mc2 := &mockConn{}
		rr := 0
		conns := []wsWriter{mc1, mc2}
		for i := 0; i < 4; i++ {
			err := roundRobinDispatch(conns, &rr, []byte("rr"))
			assert.NoError(t, err)
		}
		assert.Equal(t, 2, len(mc1.written))
		assert.Equal(t, 2, len(mc2.written))
	})

	t.Run("write failure", func(t *testing.T) {
		mc := &mockConn{fail: true}
		rr := 0
		err := roundRobinDispatch([]wsWriter{mc}, &rr, []byte("fail"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write failed")
	})
}
