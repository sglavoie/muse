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
	if err := a.validateOperand(kind); err != nil {
		return "", err
	}
	return kind, nil
}

// validateOperand checks the value carried by the one operation field the
// action sets.
func (a UITestAction) validateOperand(kind actionKind) error {
	switch kind {
	case actionKey:
		return validateKeyOperand(*a.Key)
	case actionText:
		return operandPresent(*a.Text, "text")
	case actionResize:
		return validateResizeOperand(*a.Resize)
	case actionWait:
		return validateWaitOperand(*a.Wait)
	case actionCommand:
		return operandPresent(strings.TrimSpace(*a.Command), "command")
	case actionAssert:
		return a.Assert.validate()
	}
	return nil
}

// operandPresent rejects an empty operand, naming the field it belongs to.
func operandPresent(value, field string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	return nil
}

// validateKeyOperand rejects a blank key name and one the key parser cannot
// resolve, so a bad key fails at parse time rather than mid-scenario.
func validateKeyOperand(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("key must not be empty")
	}
	if _, err := parseKey(name); err != nil {
		return fmt.Errorf("key: %w", err)
	}
	return nil
}

func validateResizeOperand(size string) error {
	if _, _, err := ParseSize(size); err != nil {
		return fmt.Errorf("resize: %w", err)
	}
	return nil
}

// validateWaitOperand accepts an empty duration, which means "use the driver's
// drain timeout"; anything else must parse.
func validateWaitOperand(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if _, err := time.ParseDuration(trimmed); err != nil {
		return fmt.Errorf("wait: invalid duration %q", raw)
	}
	return nil
}

// kind resolves which operation an action performs, requiring exactly one
// operation field to be present.
func (a UITestAction) kind() (actionKind, error) {
	kinds := a.presentKinds()
	switch len(kinds) {
	case 0:
		return "", errors.New("action has no operation (expected one of key, text, resize, wait, assert, command)")
	case 1:
		return kinds[0], nil
	default:
		return "", fmt.Errorf("action sets multiple operations: %s", joinKinds(kinds))
	}
}

// presentKinds lists the operation kinds whose field is set, in the order the
// "no operation" error names them.
func (a UITestAction) presentKinds() []actionKind {
	var kinds []actionKind
	for _, c := range []struct {
		set  bool
		kind actionKind
	}{
		{a.Key != nil, actionKey},
		{a.Text != nil, actionText},
		{a.Resize != nil, actionResize},
		{a.Wait != nil, actionWait},
		{a.Assert != nil, actionAssert},
		{a.Command != nil, actionCommand},
	} {
		if c.set {
			kinds = append(kinds, c.kind)
		}
	}
	return kinds
}

// joinKinds renders kind names for the "sets multiple ..." errors, in the order
// they were collected.
func joinKinds[T ~string](kinds []T) string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
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
	kinds := a.presentKinds()
	switch len(kinds) {
	case 0:
		return "", errors.New("assert has no expectation (expected one of contains, not_contains, view, overlay, status_contains, no_layout_issues)")
	case 1:
		// no-op: fall through to the per-kind value check below
	default:
		return "", fmt.Errorf("assert sets multiple expectations: %s", joinKinds(kinds))
	}
	if err := a.validateExpectation(kinds[0]); err != nil {
		return "", err
	}
	return kinds[0], nil
}

// presentKinds lists the assertion kinds whose field is set, in the order the
// "no expectation" error names them.
func (a *UITestAssertion) presentKinds() []assertionKind {
	var kinds []assertionKind
	for _, c := range []struct {
		set  bool
		kind assertionKind
	}{
		{a.Contains != nil, assertContains},
		{a.NotContains != nil, assertNotContains},
		{a.View != nil, assertView},
		{a.Overlay != nil, assertOverlay},
		{a.StatusContains != nil, assertStatusContains},
		{a.NoLayoutIssues != nil, assertNoLayoutIssues},
	} {
		if c.set {
			kinds = append(kinds, c.kind)
		}
	}
	return kinds
}

// validateExpectation checks the value carried by the one expectation field the
// assertion sets.
func (a *UITestAssertion) validateExpectation(kind assertionKind) error {
	switch kind {
	case assertContains:
		return operandPresent(*a.Contains, "contains")
	case assertNotContains:
		return operandPresent(*a.NotContains, "not_contains")
	case assertView:
		return operandPresent(strings.TrimSpace(*a.View), "view")
	case assertOverlay:
		return operandPresent(strings.TrimSpace(*a.Overlay), "overlay")
	case assertStatusContains:
		return operandPresent(*a.StatusContains, "status_contains")
	case assertNoLayoutIssues:
		if !*a.NoLayoutIssues {
			return errors.New("no_layout_issues must be true when set")
		}
	}
	return nil
}
