//! Tests over the generated whole-graph output, which `check.bash` produces
//! immediately before running them.

use prost::Name;
use starpc::testing::{InMemoryOpener, create_pipe_default};
use starpc::{Context, Result, Server, SrpcClient};
use tokio::task::JoinHandle;

use crate::external::{Token, ext::Marker};
use crate::graph::core::{Extra, Header, outer};
use crate::graph::legacy::r#type::Item;
use crate::graph::shared::Local;
use crate::graph::svc::{
    GraphClient, GraphClientImpl, GraphHandler, GraphServer, Request, request,
};

/// Implements every unary method, each over a different kind of type path.
struct Impl;

#[starpc::async_trait]
impl GraphServer for Impl {
    /// Returns a message of the package that spans two directories.
    async fn get(&self, _context: &Context, req: Request) -> Result<Extra> {
        Ok(Extra {
            header: req.header,
            inner: req.inner,
        })
    }

    /// Takes a nested message of another package and returns a renamed package's message.
    async fn get_inner(&self, _context: &Context, req: outer::Inner) -> Result<Item> {
        Ok(Item {
            name: req.count.to_string(),
        })
    }

    /// Takes a message from the package-prefix extern path.
    async fn get_http_url(&self, _context: &Context, req: Marker) -> Result<Local> {
        Ok(Local { value: req.id })
    }

    /// Takes and returns a message nested in the service's own package.
    async fn configure(
        &self,
        _context: &Context,
        req: request::Options,
    ) -> Result<request::Options> {
        Ok(req)
    }
}

/// Connects a generated client to the generated handler over `calls` in-memory
/// transports, one per call. The server task returns once the listener has
/// ended and every accepted call has completed.
fn connect(
    calls: usize,
) -> (
    GraphClientImpl<SrpcClient<InMemoryOpener>>,
    JoinHandle<Result<()>>,
) {
    let (client_ends, server_ends): (Vec<_>, Vec<_>) =
        (0..calls).map(|_| create_pipe_default()).unzip();
    let server = Server::new(GraphHandler::new(Impl));
    let incoming = futures::stream::iter(server_ends.into_iter().map(Ok));
    let task = tokio::spawn(async move { server.serve(incoming).await });
    let client = GraphClientImpl::new(SrpcClient::new(InMemoryOpener::new(client_ends)));
    (client, task)
}

/// Type names follow the protobuf packages and enclosing messages.
#[test]
fn type_names_follow_protobuf_names() {
    assert_eq!(Request::full_name(), "graph.svc.Request");
    assert_eq!(request::Options::full_name(), "graph.svc.Request.Options");
    assert_eq!(outer::Inner::full_name(), "graph.core.Outer.Inner");
    assert_eq!(Item::full_name(), "graph.Legacy.type.Item");
}

/// Requests carrying every kind of type path survive a round trip through the
/// generated client and handler.
#[tokio::test]
async fn unary_round_trip_resolves_every_type() {
    let (client, server) = connect(4);

    // The shared extern types are part of the request.
    let req = Request {
        header: Some(Header { id: "h".into() }),
        inner: Some(outer::Inner { count: 7 }),
        token: Some(Token { value: "t".into() }),
        marker: Some(Marker { id: "m".into() }),
        options: Some(request::Options { verbose: true }),
    };
    let extra = client.get(&req).await.unwrap();
    assert_eq!(extra.header.unwrap().id, "h");
    assert_eq!(extra.inner.unwrap().count, 7);

    // The wire name of a method is its protobuf name, not its Rust name.
    let local = client
        .get_http_url(&Marker { id: "u".into() })
        .await
        .unwrap();
    assert_eq!(local.value, "u");
    let item = client.get_inner(&outer::Inner { count: 3 }).await.unwrap();
    assert_eq!(item.name, "3");
    let options = client
        .configure(&request::Options { verbose: true })
        .await
        .unwrap();
    assert!(options.verbose);

    // Dropping the client ends its transports, which lets the server drain.
    drop(client);
    server.await.unwrap().unwrap();
}
