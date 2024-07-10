package example_test

import (
	"testing"

	"github.com/aperturerobotics/common/example"
	"github.com/aperturerobotics/common/example/other"
)

func TestExampleMsg(t *testing.T) {
	t.Run("should create an empty message", func(t *testing.T) {
		msg := &example.ExampleMsg{}
		if msg.ExampleField != "" {
			t.Errorf("Expected empty ExampleField, got %q", msg.ExampleField)
		}
		if msg.OtherMsg != nil {
			t.Error("Expected nil OtherMsg, got non-nil")
		}
	})

	t.Run("should create a message with an example field", func(t *testing.T) {
		msg := &example.ExampleMsg{ExampleField: "hello"}
		if msg.ExampleField != "hello" {
			t.Errorf("Expected ExampleField to be 'hello', got %q", msg.ExampleField)
		}
	})

	t.Run("should create a message with an other message field", func(t *testing.T) {
		other := &example_other.OtherMsg{FooField: 1}
		msg := &example.ExampleMsg{OtherMsg: other}
		if msg.OtherMsg.FooField != 1 {
			t.Errorf("Expected OtherMsg.FooField to be 1, got %d", msg.OtherMsg.FooField)
		}
	})
}
