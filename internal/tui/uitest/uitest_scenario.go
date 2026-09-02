package uitest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseSize parses a "WxH" terminal size (e.g. "100x30") into positive width
// and height. The separator is case-insensitive ('x' or 'X'). Any non-positive
// dimension, missing dimension, or non-integer component is an error.
func ParseSize(s string) (width, height int, err error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return 0, 0, errors.New("size is empty")
	}
	parts := strings.Split(strings.ToLower(raw), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("size %q must be in WxH form", s)
	}
	width, err = parsePositiveDimension(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("size %q: width: %w", s, err)
	}
	height, err = parsePositiveDimension(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("size %q: height: %w", s, err)
	}
	return width, height, nil
}

func parsePositiveDimension(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("missing value")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("not an integer: %q", s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be positive: %d", n)
	}
	return n, nil
}

// ParseScenario strictly decodes scenario JSON. Unknown fields (at any depth),
// trailing content, and structural problems are rejected, then the decoded
// scenario is validated. It never partially applies a bad scenario.
func ParseScenario(data []byte) (UITestScenario, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var s UITestScenario
	if err := dec.Decode(&s); err != nil {
		return UITestScenario{}, fmt.Errorf("decode scenario: %w", err)
	}
	if dec.More() {
		return UITestScenario{}, errors.New("decode scenario: unexpected trailing content after JSON value")
	}
	if err := s.validate(); err != nil {
		return UITestScenario{}, err
	}
	return s, nil
}

// validate checks scenario-level invariants: a non-empty name, a well-formed
// optional size, and at least one valid action.
func (s UITestScenario) validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("scenario: name is required")
	}
	if s.Size != "" {
		if _, _, err := ParseSize(s.Size); err != nil {
			return fmt.Errorf("scenario %q: %w", s.Name, err)
		}
	}
	if len(s.Actions) == 0 {
		return fmt.Errorf("scenario %q: must contain at least one action", s.Name)
	}
	for i, a := range s.Actions {
		if _, err := a.validate(); err != nil {
			return fmt.Errorf("scenario %q: action %d: %w", s.Name, i, err)
		}
	}
	return nil
}

// validate checks that the action sets exactly one recognized operation field
// and that the field's value is well-formed. It returns the resolved action
// kind so callers can dispatch without re-inspecting the struct.
func (a UITestAction) validate() (actionKind, error) {
	kind, err := a.kind()
	if err != nil {
		return "", err
	}
	switch kind {
	case actionKey:
		if strings.TrimSpace(*a.Key) == "" {
			return "", errors.New("key must not be empty")
		}
		if _, err := parseKey(*a.Key); err != nil {
			return "", fmt.Errorf("key: %w", err)
		}
	case actionText:
		if *a.Text == "" {
			return "", errors.New("text must not be empty")
		}
	case actionResize:
		if _, _, err := ParseSize(*a.Resize); err != nil {
			return "", fmt.Errorf("resize: %w", err)
		}
	case actionWait:
		if strings.TrimSpace(*a.Wait) != "" {
			if _, err := time.ParseDuration(strings.TrimSpace(*a.Wait)); err != nil {
				return "", fmt.Errorf("wait: invalid duration %q", *a.Wait)
			}
		}
	case actionCommand:
		if strings.TrimSpace(*a.Command) == "" {
			return "", errors.New("command must not be empty")
		}
	case actionAssert:
		if err := a.Assert.validate(); err != nil {
			return "", err
		}
	}
	return kind, nil
}

// kind resolves which operation an action performs, requiring exactly one
// operation field to be present.
func (a UITestAction) kind() (actionKind, error) {
	var (
		kinds []actionKind
	)
	if a.Key != nil {
		kinds = append(kinds, actionKey)
	}
	if a.Text != nil {
		kinds = append(kinds, actionText)
	}
	if a.Resize != nil {
		kinds = append(kinds, actionResize)
	}
	if a.Wait != nil {
		kinds = append(kinds, actionWait)
	}
	if a.Assert != nil {
		kinds = append(kinds, actionAssert)
	}
	if a.Command != nil {
		kinds = append(kinds, actionCommand)
	}
	switch len(kinds) {
	case 0:
		return "", errors.New("action has no operation (expected one of key, text, resize, wait, assert, command)")
	case 1:
		return kinds[0], nil
	default:
		names := make([]string, len(kinds))
		for i, k := range kinds {
			names[i] = string(k)
		}
		return "", fmt.Errorf("action sets multiple operations: %s", strings.Join(names, ", "))
	}
}

// validate checks that the assertion sets exactly one recognized kind and that
// its value is usable. It returns the resolved assertion kind.
func (a *UITestAssertion) validate() error {
	_, err := a.kind()
	return err
}

// kind resolves which assertion this is, requiring exactly one field present.
func (a *UITestAssertion) kind() (assertionKind, error) {
	if a == nil {
		return "", errors.New("assert must not be null")
	}
	var kinds []assertionKind
	if a.Contains != nil {
		kinds = append(kinds, assertContains)
	}
	if a.NotContains != nil {
		kinds = append(kinds, assertNotContains)
	}
	if a.View != nil {
		kinds = append(kinds, assertView)
	}
	if a.Overlay != nil {
		kinds = append(kinds, assertOverlay)
	}
	if a.StatusContains != nil {
		kinds = append(kinds, assertStatusContains)
	}
	if a.NoLayoutIssues != nil {
		kinds = append(kinds, assertNoLayoutIssues)
	}
	switch len(kinds) {
	case 0:
		return "", errors.New("assert has no expectation (expected one of contains, not_contains, view, overlay, status_contains, no_layout_issues)")
	case 1:
		// no-op: fall through to per-kind checks below
	default:
		names := make([]string, len(kinds))
		for i, k := range kinds {
			names[i] = string(k)
		}
		return "", fmt.Errorf("assert sets multiple expectations: %s", strings.Join(names, ", "))
	}

	switch kinds[0] {
	case assertContains:
		if *a.Contains == "" {
			return "", errors.New("contains must not be empty")
		}
	case assertNotContains:
		if *a.NotContains == "" {
			return "", errors.New("not_contains must not be empty")
		}
	case assertView:
		if strings.TrimSpace(*a.View) == "" {
			return "", errors.New("view must not be empty")
		}
	case assertOverlay:
		if strings.TrimSpace(*a.Overlay) == "" {
			return "", errors.New("overlay must not be empty")
		}
	case assertStatusContains:
		if *a.StatusContains == "" {
			return "", errors.New("status_contains must not be empty")
		}
	case assertNoLayoutIssues:
		if !*a.NoLayoutIssues {
			return "", errors.New("no_layout_issues must be true when set")
		}
	}
	return kinds[0], nil
}
