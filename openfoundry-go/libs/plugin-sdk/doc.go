// Package pluginsdk mirrors libs/plugin-sdk from the Rust workspace.
//
// The Rust crate is currently a placeholder — its `src/lib.rs` is
// empty and the package only declares dependencies (serde,
// serde_json, thiserror) for the future Rust + WASM SDK that will
// host OpenFoundry connectors, transforms, and widgets.
//
// This Go package is the matching placeholder. When the Rust SDK
// surface lands, it will be ported here 1:1.
package pluginsdk
