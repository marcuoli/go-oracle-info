package services

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// Only the REMOTE_LISTENER child of a named network is relevant. A local or
// forward listener can use a DNS name unrelated to the remote registration.
func remoteNetworkTargets(value string) ([]string, bool) {
	value = unquoteNetValue(value)
	groups, ok := netGroups(value)
	if !ok {
		return nil, false
	}
	var targets []string
	for _, group := range groups {
		network := group.value
		if group.key != "" {
			// Some configurations omit the extra wrapper around the fields.
			network = value
		}
		fields, ok := netGroups(network)
		if !ok {
			return nil, false
		}
		named := false
		for _, field := range fields {
			switch field.key {
			case "NAME":
				named = field.value != ""
			case "REMOTE_LISTENER":
				targets = append(targets, field.value)
			case "LOCAL_LISTENER", "FORWARD_LISTENER":
			default:
				return nil, false
			}
		}
		if !named {
			return nil, false
		}
		if group.key != "" {
			break
		}
	}
	return targets, true
}

// registrationHosts recognizes explicit Easy Connect host:port values and
// address descriptors. Any unresolved member (including an IP) prevents an
// inference that all configured targets refer to one DNS name.
func registrationHosts(value string, depth int) []string {
	if depth >= 8 {
		return nil
	}
	value = unquoteNetValue(value)
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(value, "(") {
		var hosts []string
		for _, target := range strings.Split(value, ",") {
			host, port, err := net.SplitHostPort(strings.TrimSpace(target))
			if err != nil {
				return nil
			}
			host = registrationDNSName(host, port)
			if host == "" {
				return nil
			}
			hosts = append(hosts, host)
		}
		return hosts
	}

	groups, ok := netGroups(value)
	if !ok {
		return nil
	}
	var hosts []string
	for _, group := range groups {
		switch group.key {
		case "DESCRIPTION_LIST", "DESCRIPTION", "ADDRESS_LIST":
			children := registrationHosts(group.value, depth+1)
			if len(children) == 0 {
				return nil
			}
			hosts = append(hosts, children...)
		case "ADDRESS":
			host := addressDNSName(group.value)
			if host == "" {
				return nil
			}
			hosts = append(hosts, host)
		case "CONNECT_DATA", "LOAD_BALANCE", "FAILOVER", "SOURCE_ROUTE":
			// These attributes carry no registration address.
		default:
			return nil
		}
	}
	return hosts
}

func addressDNSName(value string) string {
	fields, ok := netGroups(value)
	if !ok || len(fields) != 3 {
		return ""
	}
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		values[field.key] = unquoteNetValue(field.value)
	}
	protocol := strings.ToLower(values["PROTOCOL"])
	if protocol != "tcp" && protocol != "tcps" {
		return ""
	}
	return registrationDNSName(values["HOST"], values["PORT"])
}

func registrationDNSName(host, port string) string {
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return ""
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return ""
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if len(host) > 253 {
		return ""
	}
	hasLetter := false
	for label := range strings.SplitSeq(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ""
		}
		for _, char := range label {
			switch {
			case char >= 'a' && char <= 'z':
				hasLetter = true
			case char >= '0' && char <= '9', char == '-':
			default:
				return ""
			}
		}
	}
	if !hasLetter {
		return ""
	}
	return host
}

type netGroup struct{ key, value string }

// netGroups splits a sequence of balanced Oracle Net groups at its outer
// level. Quoted values may contain parentheses or commas.
func netGroups(value string) ([]netGroup, bool) {
	var groups []netGroup
	allowComma := false
	for value = strings.TrimSpace(value); value != ""; value = strings.TrimSpace(value) {
		if value[0] == ',' && allowComma {
			value = strings.TrimSpace(value[1:])
		}
		if !strings.HasPrefix(value, "(") {
			return nil, false
		}
		var depth, end int
		var quote byte
		for i := range len(value) {
			char := value[i]
			switch {
			case quote != 0:
				if char == quote {
					quote = 0
				}
			case char == '"' || char == '\'':
				quote = char
			case char == '(':
				depth++
			case char == ')':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end != 0 {
				break
			}
		}
		if end == 0 {
			return nil, false
		}
		content := strings.TrimSpace(value[1:end])
		if strings.HasPrefix(content, "(") {
			groups = append(groups, netGroup{value: content})
		} else {
			key, payload, ok := strings.Cut(content, "=")
			if !ok || strings.TrimSpace(key) == "" {
				return nil, false
			}
			groups = append(groups, netGroup{strings.ToUpper(strings.TrimSpace(key)), strings.TrimSpace(payload)})
		}
		value, allowComma = value[end+1:], true
	}
	return groups, len(groups) > 0
}

func unquoteNetValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		return strings.TrimSpace(value[1 : len(value)-1])
	}
	return value
}
