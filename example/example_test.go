package example_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/common/example"
	example_other "github.com/aperturerobotics/common/example/other"
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

// TestRustGeneratedCode validates that the generated Rust protobuf code compiles and passes tests.
func TestRustGeneratedCode(t *testing.T) {
	// Check if cargo is available
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo not found, skipping Rust validation")
	}

	// Get the directory containing the Cargo.toml
	exampleDir, err := filepath.Abs(filepath.Dir("."))
	if err != nil {
		t.Fatalf("failed to get example dir: %v", err)
	}

	// Check if Cargo.toml exists
	cargoToml := filepath.Join(exampleDir, "Cargo.toml")
	if _, err := os.Stat(cargoToml); os.IsNotExist(err) {
		t.Skip("Cargo.toml not found, skipping Rust validation")
	}

	t.Run("cargo check", func(t *testing.T) {
		cmd := exec.Command("cargo", "check")
		cmd.Dir = exampleDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("cargo check failed: %v\n%s", err, output)
		}
	})

	t.Run("cargo test", func(t *testing.T) {
		cmd := exec.Command("cargo", "test")
		cmd.Dir = exampleDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("cargo test failed: %v\n%s", err, output)
		}
		t.Logf("cargo test output:\n%s", output)
	})
}
