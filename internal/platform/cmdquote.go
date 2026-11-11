package platform

import "strings"

var cmdMeta = strings.NewReplacer("^", "^^", "&", "^&", "|", "^|", "<", "^<", ">", "^>", "(", "^(", ")", "^)", `"`, `^"`, "%", "%^", "!", "^!")

func quoteCmdArg(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(s)
	return cmdMeta.Replace(`"` + escapeBackslashes(s) + `"`)
}

func escapeBackslashes(s string) string {
	var b strings.Builder
	slashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			slashes++
			continue
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes*2+1))
			b.WriteRune(r)
		default:
			b.WriteString(strings.Repeat(`\`, slashes))
			b.WriteRune(r)
		}
		slashes = 0
	}
	b.WriteString(strings.Repeat(`\`, slashes*2))
	return b.String()
}
