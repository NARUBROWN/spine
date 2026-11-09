package input

import (
	"encoding"
	"fmt"
	"reflect"
	"strings"
)

type headerSyntax struct {
	list       bool
	single     bool
	opaque     bool
	comments   bool
	angles     bool
	customList bool
	cookie     bool
}

func syntaxFor(name string) headerSyntax {
	switch strings.ToLower(name) {
	case "if-match", "if-none-match":
		return headerSyntax{list: true, opaque: true}
	case "via":
		return headerSyntax{list: true, comments: true}
	case "link":
		return headerSyntax{list: true, angles: true}
	case "www-authenticate", "proxy-authenticate":
		return headerSyntax{list: true, customList: true}
	case "accept", "accept-charset", "accept-encoding", "accept-language", "allow", "cache-control", "connection", "content-encoding", "content-language", "forwarded", "pragma", "te", "trailer", "transfer-encoding", "upgrade", "vary", "access-control-request-headers", "access-control-allow-headers", "access-control-allow-methods", "access-control-expose-headers", "timing-allow-origin", "sec-websocket-extensions", "sec-websocket-protocol", "x-forwarded-for", "x-forwarded-host":
		return headerSyntax{list: true}
	case "cookie":
		return headerSyntax{single: true, cookie: true}
	case "authorization", "proxy-authorization", "content-type", "content-length", "host", "date", "if-modified-since", "if-unmodified-since", "if-range", "etag", "range", "content-range", "origin", "referer", "user-agent", "server", "location", "content-location", "max-forwards", "set-cookie":
		return headerSyntax{single: true}
	}
	return headerSyntax{}
}

func headerList(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && !reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}

func checkHeaderType(name string, t reflect.Type) error {
	syntax := syntaxFor(name)
	if headerList(t) && (syntax.single || syntax.customList) {
		return fmt.Errorf("header %s does not support generic list binding; use a scalar or TextUnmarshaler", name)
	}
	return nil
}

func headerValues(name string, t reflect.Type, raw []string) ([]string, error) {
	syntax := syntaxFor(name)
	if headerList(t) {
		if err := checkHeaderType(name, t); err != nil {
			return nil, err
		}
		return splitHeaderList(strings.Join(raw, ", "), syntax)
	}
	if syntax.cookie {
		return []string{strings.Join(raw, "; ")}, nil
	}
	if syntax.list {
		return []string{strings.Join(raw, ", ")}, nil
	}
	if len(raw) > 1 {
		return nil, fmt.Errorf("multiple values for header field %s", name)
	}
	return raw, nil
}

// splitHeaderList는 바깥 구분자만 나누며 각 원소의 프로토콜 표현을 보존합니다.
// 따옴표 안의 쉼표와 Link URI, Via 주석은 원소를 나누지 않습니다.
func splitHeaderList(raw string, syntax headerSyntax) ([]string, error) {
	var values []string
	start, comments := 0, 0
	quoted, escaped, angle := false, false, false
	appendValue := func(end int) {
		if value := strings.Trim(raw[start:end], " \t"); value != "" {
			values = append(values, value)
		}
	}
	for index := 0; index < len(raw); index++ {
		c := raw[index]
		if escaped {
			escaped = false
			continue
		}
		if quoted {
			if c == '\\' && !syntax.opaque {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if comments > 0 {
			if c == '\\' {
				escaped = true
			} else if c == '(' {
				comments++
			} else if c == ')' {
				comments--
			}
			continue
		}
		if angle {
			if c == '>' {
				angle = false
			}
			continue
		}
		switch {
		case c == '"':
			quoted = true
		case c == '(' && syntax.comments:
			comments = 1
		case c == '<' && syntax.angles:
			angle = true
		case c == ',':
			appendValue(index)
			start = index + 1
		}
	}
	if quoted || escaped || comments > 0 || angle {
		return nil, fmt.Errorf("invalid header list syntax")
	}
	appendValue(len(raw))
	if syntax.opaque {
		for _, value := range values {
			if value == "*" {
				if len(values) != 1 {
					return nil, fmt.Errorf("invalid entity tag list")
				}
				continue
			}
			tag := strings.TrimPrefix(value, "W/")
			if len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
				return nil, fmt.Errorf("invalid entity tag list")
			}
			for i := 1; i < len(tag)-1; i++ {
				if c := tag[i]; c != '!' && !(c >= '#' && c <= '~') && c < 0x80 {
					return nil, fmt.Errorf("invalid entity tag list")
				}
			}
		}
	}
	return values, nil
}
