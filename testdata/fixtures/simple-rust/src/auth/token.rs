pub struct Token(pub String);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn wraps() {
        assert_eq!(Token("a".into()).0, "a");
    }
}
