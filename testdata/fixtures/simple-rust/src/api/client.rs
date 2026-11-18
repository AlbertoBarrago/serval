use crate::auth::middleware;
use std::fmt::Write;

pub fn call() -> bool {
    let mut s = String::new();
    let _ = write!(s, "{}", 1);
    middleware::check(&s)
}
