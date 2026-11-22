package faults

import (
	"crypto/rand"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/faultline-proxy/faultline/pkg/models"
)

// Matcher evaluates incoming HTTP requests against FaultRules.
type Matcher struct {
	regexMu    sync.RWMutex
	regexCache map[string]*regexp.Regexp
}

// NewMatcher creates a thread-safe rule matcher.
func NewMatcher() *Matcher {
	return &Matcher{
		regexCache: make(map[string]*regexp.Regexp),
	}
}

// Matches checks if an incoming HTTP request satisfies all criteria of a FaultRule.
func (m *Matcher) Matches(req *http.Request, clientIP string, rule *models.FaultRule) bool {
	if !rule.IsEnabled {
		return false
	}

	// 1. HTTP Method Match
	if len(rule.MatchMethods) > 0 {
		methodMatched := false
		reqMethod := strings.ToUpper(req.Method)
		for _, m := range rule.MatchMethods {
			if strings.ToUpper(strings.TrimSpace(m)) == reqMethod {
				methodMatched = true
				break
			}
		}
		if !methodMatched {
			return false
		}
	}

	// 2. Path Pattern Match (Regex or Prefix)
	if rule.MatchPathPattern != "" {
		re, err := m.getCompiledRegex(rule.MatchPathPattern)
		if err == nil {
			if !re.MatchString(req.URL.Path) {
				return false
			}
		} else {
			// Fallback to prefix match
			if !strings.HasPrefix(req.URL.Path, rule.MatchPathPattern) {
				return false
			}
		}
	}

	// 3. Header Matchers (Key: regex)
	if len(rule.MatchHeaders) > 0 {
		for key, pattern := range rule.MatchHeaders {
			val := req.Header.Get(key)
			if pattern == "" {
				// Empty pattern means header must simply exist
				if val == "" {
					return false
				}
				continue
			}
			re, err := m.getCompiledRegex(pattern)
			if err != nil || !re.MatchString(val) {
				return false
			}
		}
	}

	// 4. Query Parameter Matchers
	if len(rule.MatchQueryParams) > 0 {
		q := req.URL.Query()
		for key, pattern := range rule.MatchQueryParams {
			val := q.Get(key)
			if pattern == "" {
				if val == "" {
					return false
				}
				continue
			}
			re, err := m.getCompiledRegex(pattern)
			if err != nil || !re.MatchString(val) {
				return false
			}
		}
	}

	// 5. Client IP / CIDR Matchers
	if len(rule.MatchClientIPs) > 0 && clientIP != "" {
		matchedIP := false
		parsedClientIP := net.ParseIP(clientIP)
		for _, target := range rule.MatchClientIPs {
			target = strings.TrimSpace(target)
			if strings.Contains(target, "/") {
				_, cidr, err := net.ParseCIDR(target)
				if err == nil && parsedClientIP != nil && cidr.Contains(parsedClientIP) {
					matchedIP = true
					break
				}
			} else {
				if target == clientIP {
					matchedIP = true
					break
				}
			}
		}
		if !matchedIP {
			return false
		}
	}

	// 6. Probability Check
	if rule.Probability <= 0.0 {
		return false
	}
	if rule.Probability < 1.0 {
		dice := CryptoFloat64()
		if dice > rule.Probability {
			return false
		}
	}

	return true
}

func (m *Matcher) getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	m.regexMu.RLock()
	re, ok := m.regexCache[pattern]
	m.regexMu.RUnlock()
	if ok {
		return re, nil
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	m.regexMu.Lock()
	m.regexCache[pattern] = compiled
	m.regexMu.Unlock()
	return compiled, nil
}

// CryptoFloat64 returns a cryptographically secure random float in [0.0, 1.0).
func CryptoFloat64() float64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return 0.5 // Safe fallback
	}
	return float64(n.Int64()) / 1000000.0
}
