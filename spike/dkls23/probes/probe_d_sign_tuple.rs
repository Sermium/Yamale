//! D: does sign::run return Result<(Signature, RecoveryId)>?
//!
//! sign.rs destructures `let (sign, recid) = ...`, but whether the Result is
//! unwrapped before or after that could not be read. This is the shape the
//! sidecar's signing path is built around, so it is worth its own probe.
mod common;

use common::shared::{gen_keyshares, setup_dsg};
use rand::Rng;
use rand_chacha::ChaCha20Rng;
use rand_core::SeedableRng;
use sl_dkls23::sign;
use sl_mpc_mate::coord::SimpleMessageRelay;

#[tokio::main]
async fn main() {
    let shares = gen_keyshares(2, 3).await;
    let coord = SimpleMessageRelay::new();
    let mut rnd = ChaCha20Rng::from_entropy();

    for setup in setup_dsg(&shares[0..2], "m") {
        let (_signature, _recid) = sign::run(setup, rnd.gen(), coord.connect())
            .await
            .unwrap();
    }
}
