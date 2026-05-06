// Package geospatialcore mirrors libs/geospatial-core from the Rust
// workspace.
//
// The Rust crate is currently a placeholder: `src/lib.rs` is empty
// and the four declared modules (`geometry`, `h3`, `tile`, `wkt`)
// are also empty stub files. The crate is reserved for the future
// geospatial primitives — H3 indexing, WKT parsing, and MVT tile
// encoding — described in its Cargo.toml.
//
// This Go package is the matching placeholder. When the Rust crate
// gets real source, the modules will be ported here 1:1, likely as
// subpackages mirroring the Rust module layout.
package geospatialcore
