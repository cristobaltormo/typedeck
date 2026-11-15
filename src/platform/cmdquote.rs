pub fn quote_cmd_arg(s: &str) -> String {
    let s = s.replace("\r\n", " ").replace(['\r', '\n'], " ");
    let quoted = format!("\"{}\"", escape_backslashes(&s));
    let mut out = String::with_capacity(quoted.len() * 2);
    for c in quoted.chars() {
        match c {
            '^' => out.push_str("^^"),
            '&' | '|' | '<' | '>' | '(' | ')' | '"' | '!' => {
                out.push('^');
                out.push(c);
            }
            '%' => out.push_str("%^"),
            c => out.push(c),
        }
    }
    out
}

fn escape_backslashes(s: &str) -> String {
    let mut b = String::with_capacity(s.len());
    let mut slashes = 0;
    for c in s.chars() {
        match c {
            '\\' => {
                slashes += 1;
                continue;
            }
            '"' => {
                b.push_str(&"\\".repeat(slashes * 2 + 1));
                b.push(c);
            }
            c => {
                b.push_str(&"\\".repeat(slashes));
                b.push(c);
            }
        }
        slashes = 0;
    }
    b.push_str(&"\\".repeat(slashes * 2));
    b
}
