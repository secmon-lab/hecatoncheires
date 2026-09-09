package notiontool

import (
	"errors"
	"fmt"
	"strings"
)

// schemaNamesInMessageMax bounds how many property names a rejection lists. A
// data source in production can hold dozens of columns, and the message is read
// by a model whose context the rest of the answer also has to fit in.
const schemaNamesInMessageMax = 30

// describePropertiesMax bounds how many properties one call may ask the choices
// of. Asking for all of them is what the per-property request exists to avoid.
const describePropertiesMax = 10

// rejection reports that the agent's arguments cannot be turned into a Notion
// filter — an unknown property name, an operator its type does not accept, a
// value that is not of the type the property holds.
//
// The tools answer it as a SUCCESSFUL result carrying this message, not as an
// error. A wrong property name is the model's own mistake and something it can
// repair on the next call, while the strategies report every tool failure to
// Sentry — returning an error would file an issue per attempt. That is the same
// reasoning that tags a 404 benign in newAPIError.
//
// Discriminate with errors.As, never by matching the message.
type rejection struct {
	message string
}

func (e *rejection) Error() string { return e.message }

func rejectf(format string, a ...any) error {
	return &rejection{message: fmt.Sprintf(format, a...)}
}

// rejectionMessage returns the message to show the agent, and whether err is a
// rejection at all. A non-rejection error belongs to the caller to propagate.
func rejectionMessage(err error) (string, bool) {
	var r *rejection
	if errors.As(err, &r) {
		return r.message, true
	}
	return "", false
}

// The operator sets, per property type. Every list is exactly what Notion
// accepts for that type's condition; an operator outside it is a 400 from
// Notion, which is why the sets are enforced here instead.
var (
	textOperators        = []string{"equals", "does_not_equal", "contains", "does_not_contain", "starts_with", "ends_with", "is_empty", "is_not_empty"}
	numberOperators      = []string{"equals", "does_not_equal", "greater_than", "greater_than_or_equal_to", "less_than", "less_than_or_equal_to", "is_empty", "is_not_empty"}
	uniqueIDOperators    = []string{"equals", "does_not_equal", "greater_than", "greater_than_or_equal_to", "less_than", "less_than_or_equal_to"}
	checkboxOperators    = []string{"equals", "does_not_equal"}
	choiceOperators      = []string{"equals", "does_not_equal", "is_empty", "is_not_empty"}
	multiSelectOperators = []string{"contains", "does_not_contain", "is_empty", "is_not_empty"}
	dateOperators        = []string{"equals", "before", "after", "on_or_before", "on_or_after", "is_empty", "is_not_empty", "past_week", "past_month", "past_year", "this_week", "next_week", "next_month", "next_year"}
	// A created_time / last_edited_time property always holds a value, so
	// Notion's timestamp filter has no emptiness conditions.
	timestampOperators = []string{"equals", "before", "after", "on_or_before", "on_or_after", "past_week", "past_month", "past_year", "this_week", "next_week", "next_month", "next_year"}
	peopleOperators    = []string{"contains", "does_not_contain", "is_empty", "is_not_empty"}
	// created_by / last_edited_by are never empty either.
	byOperators       = []string{"contains", "does_not_contain"}
	relationOperators = []string{"contains", "does_not_contain", "is_empty", "is_not_empty"}
	filesOperators    = []string{"is_empty", "is_not_empty"}
)

// derivedOperators is what a formula or rollup property reports: which of these
// it actually accepts depends on the result type the caller declares, and that
// is not in the schema (Notion's schema carries a formula's expression and a
// rollup's aggregation, never their result type).
var derivedOperators = unionOperators(textOperators, numberOperators, checkboxOperators, dateOperators)

// valueKind says how a condition's value, which arrives as text, is coerced
// before it is sent.
type valueKind int

const (
	valueString valueKind = iota
	valueNumber
	valueBool
	valueDate
	valueNone
)

// conditionKind describes how one property type's conditions are written.
type conditionKind struct {
	// key is the condition key Notion expects. It matches the property type for
	// most types, but not all: a title is filtered as rich_text, and
	// created_by / last_edited_by as people.
	key string
	// operators is what that key accepts.
	operators []string
	// value is how the condition's value is coerced.
	value valueKind
	// timestamp names Notion's top-level timestamp filter, which replaces the
	// property filter entirely for created_time and last_edited_time. Empty for
	// every other type.
	timestamp string
}

// conditionKindFor maps a property type onto its condition shape. The second
// return value is false for a type Notion cannot filter on, and for a type this
// package does not know — including formula and rollup, whose shape depends on a
// declared result type and is built separately.
func conditionKindFor(propType string) (conditionKind, bool) {
	switch propType {
	// A title property is filtered with the rich_text condition; Notion has no
	// "title" condition key (jomei/notionapi's PropertyFilter has no such field
	// either).
	case propTypeTitle, propTypeRichText, propTypeURL, propTypeEmail, propTypePhoneNumber:
		return conditionKind{key: propTypeRichText, operators: textOperators, value: valueString}, true
	case propTypeNumber:
		return conditionKind{key: propTypeNumber, operators: numberOperators, value: valueNumber}, true
	case propTypeUniqueID:
		return conditionKind{key: propTypeUniqueID, operators: uniqueIDOperators, value: valueNumber}, true
	case propTypeCheckbox:
		return conditionKind{key: propTypeCheckbox, operators: checkboxOperators, value: valueBool}, true
	case propTypeSelect:
		return conditionKind{key: propTypeSelect, operators: choiceOperators, value: valueString}, true
	case propTypeStatus:
		return conditionKind{key: propTypeStatus, operators: choiceOperators, value: valueString}, true
	case propTypeMultiSelect:
		return conditionKind{key: propTypeMultiSelect, operators: multiSelectOperators, value: valueString}, true
	case propTypeDate:
		return conditionKind{key: propTypeDate, operators: dateOperators, value: valueDate}, true
	case propTypeCreatedTime:
		return conditionKind{key: propTypeCreatedTime, operators: timestampOperators, value: valueDate, timestamp: propTypeCreatedTime}, true
	case propTypeLastEditedTime:
		return conditionKind{key: propTypeLastEditedTime, operators: timestampOperators, value: valueDate, timestamp: propTypeLastEditedTime}, true
	case propTypePeople:
		return conditionKind{key: propTypePeople, operators: peopleOperators, value: valueString}, true
	case propTypeCreatedBy, propTypeLastEditedBy:
		return conditionKind{key: propTypePeople, operators: byOperators, value: valueString}, true
	case propTypeRelation:
		return conditionKind{key: propTypeRelation, operators: relationOperators, value: valueString}, true
	case propTypeFiles:
		return conditionKind{key: propTypeFiles, operators: filesOperators, value: valueNone}, true
	}
	return conditionKind{}, false
}

// operatorsFor is what the agent is told a property type accepts. Nil means the
// type cannot be filtered on at all.
func operatorsFor(propType string) []string {
	if kind, ok := conditionKindFor(propType); ok {
		return kind.operators
	}
	if propType == propTypeFormula || propType == propTypeRollup {
		return derivedOperators
	}
	return nil
}

// operatorsByType reports the operators for the types this data source actually
// uses. Reporting the whole catalog instead would spend context on types the
// caller cannot name.
func operatorsByType(ds *DataSource) map[string][]string {
	out := make(map[string][]string)
	for _, p := range ds.Properties {
		if _, seen := out[p.Type]; seen {
			continue
		}
		if ops := operatorsFor(p.Type); len(ops) > 0 {
			out[p.Type] = ops
		}
	}
	return out
}

// resolveProperty finds the property the agent named, and returns a rejection
// naming the alternatives when it cannot.
func resolveProperty(ds *DataSource, name string) (PropertySchema, error) {
	if name == "" {
		return PropertySchema{}, rejectf("a property name is required; %s", availableProperties(ds))
	}

	for _, p := range ds.Properties {
		if p.Name == name {
			return p, nil
		}
	}

	// Fall back to ignoring case. The agent reads these names out of
	// property_schema but also retypes them, and a case-only difference is
	// unambiguous as long as exactly one property matches.
	var matches []PropertySchema
	for _, p := range ds.Properties {
		if strings.EqualFold(p.Name, name) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return PropertySchema{}, rejectf("property %q does not exist on this data source; %s", name, availableProperties(ds))
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, m.Name)
		}
		return PropertySchema{}, rejectf("property %q matches several properties when case is ignored (%s); use the exact name", name, strings.Join(names, ", "))
	}
}

// resolveProperties resolves a list of property names, dropping duplicates and
// keeping the order the agent asked for.
func resolveProperties(ds *DataSource, names []string) ([]PropertySchema, error) {
	if len(names) == 0 {
		return nil, nil
	}
	out := make([]PropertySchema, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		prop, err := resolveProperty(ds, name)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[prop.ID]; ok {
			continue
		}
		seen[prop.ID] = struct{}{}
		out = append(out, prop)
	}
	return out, nil
}

// availableProperties renders the "here is what you can name" half of a
// rejection.
func availableProperties(ds *DataSource) string {
	if len(ds.Properties) == 0 {
		return "this data source has no properties"
	}
	names := make([]string, 0, len(ds.Properties))
	for _, p := range ds.Properties {
		if len(names) == schemaNamesInMessageMax {
			return fmt.Sprintf("available properties: %s (and %d more)", strings.Join(names, ", "), len(ds.Properties)-len(names))
		}
		names = append(names, p.Name)
	}
	return "available properties: " + strings.Join(names, ", ")
}

// parseNameList reads an array-of-strings argument. A shape that is not an
// array of strings is a rejection rather than an error: it is the model's
// mistake and it can be restated.
func parseNameList(args map[string]any, key string, max int) ([]string, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, rejectf("%s must be an array of property names", key)
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		name, ok := item.(string)
		if !ok {
			return nil, rejectf("%s[%d] must be a property name written as text", key, i)
		}
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		out = append(out, name)
	}
	if len(out) > max {
		return nil, rejectf("%s holds %d entries; at most %d are accepted", key, len(out), max)
	}
	return out, nil
}

func unionOperators(sets ...[]string) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, set := range sets {
		for _, op := range set {
			if _, ok := seen[op]; ok {
				continue
			}
			seen[op] = struct{}{}
			out = append(out, op)
		}
	}
	return out
}
