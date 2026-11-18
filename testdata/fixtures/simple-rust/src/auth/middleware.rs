use super::token::Token;

pub fn check(raw: &str) -> bool {
    !Token(raw.to_string()).0.is_empty()
}
