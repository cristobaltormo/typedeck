const TEMPLATE: &str = include_str!("task.xml");

fn xml_text(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '&' => out.push_str("&amp;"),
            '<' => out.push_str("&lt;"),
            '>' => out.push_str("&gt;"),
            '"' => out.push_str("&#34;"),
            '\'' => out.push_str("&#39;"),
            '\t' => out.push_str("&#x9;"),
            '\n' => out.push_str("&#xA;"),
            '\r' => out.push_str("&#xD;"),
            c => out.push(c),
        }
    }
    out
}

pub fn task_xml(user: &str, exe: &str) -> String {
    TEMPLATE.replace("{USER}", &xml_text(user)).replace("{EXE}", &xml_text(exe))
}

pub fn utf16_with_bom(s: &str) -> Vec<u8> {
    std::iter::once(0xFEFFu16).chain(s.encode_utf16()).flat_map(u16::to_le_bytes).collect()
}
