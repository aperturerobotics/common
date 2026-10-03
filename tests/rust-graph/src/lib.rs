//! Checks that the generated whole-graph output compiles and resolves its types.

include!(concat!(env!("CARGO_MANIFEST_DIR"), "/modules.rs"));

/// Types that the extern paths of package.json name.
pub mod external {
    /// Token is the exact extern path of graph.shared.Token.
    #[derive(Clone, PartialEq, Eq, Hash, prost::Message)]
    pub struct Token {
        #[prost(string, tag = "1")]
        /// Value carried across the exact extern mapping.
        pub value: String,
    }

    /// Types under the package-prefix extern path of graph.ext.
    pub mod ext {
        /// Marker is graph.ext.Marker.
        #[derive(Clone, PartialEq, Eq, Hash, prost::Message)]
        pub struct Marker {
            #[prost(string, tag = "1")]
            /// Identity carried across the package extern mapping.
            pub id: String,
        }
    }
}

#[cfg(test)]
#[path = "lib_test.rs"]
mod tests;
