// lib.rs - Validates generated protobuf Rust code compiles correctly
// This file is used by tests to verify the prost-generated code.
//
// Generated .pb.rs files are placed in the source directory alongside their
// corresponding .proto files during code generation.

pub mod other {
    include!("other/other.pb.rs");
}

include!("example.pb.rs");

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_example_msg() {
        let msg = ExampleMsg {
            example_field: "hello".to_string(),
            other_msg: Some(other::OtherMsg { foo_field: 42 }),
        };
        assert_eq!(msg.example_field, "hello");
        assert_eq!(msg.other_msg.unwrap().foo_field, 42);
    }

    #[test]
    fn test_echo_msg() {
        let msg = EchoMsg {
            body: "test".to_string(),
        };
        assert_eq!(msg.body, "test");
    }
}
