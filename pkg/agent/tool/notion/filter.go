package notiontool

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// schemaNamesInMessageMax bounds how many property names a rejection lists. A
// data source in production can hold dozens of columns, and the message is read
// by a model whose context the rest of the answer also has to fit in.
const schemaNamesInMessageMax = 30

// describePropertiesMax bounds how many properties one call may ask the choices
// of. Asking for all of them is what the per-property request exists to avoid.
const describePropertiesMax = 10

// The bounds on one search request. They exist to keep a single call's filter
// legible in the run timeline and to stop a model from spending a round
// building a query nobody can read, not because Notion refuses more.
const (
	queryKeywordsMax    = 5
	searchPropertiesMax = 5
	filterConditionsMax = 25
	filterGroupsMax     = 5
	sortsMax            = 5
	returnPropertiesMax = 15
)

const (
	operatorAnd = "and"
	operatorOr  = "or"

	operatorEquals   = "equals"
	operatorContains = "contains"

	directionAscending  = "ascending"
	directionDescending = "descending"
)

// The value types a caller declares for a formula or rollup property, whose
// result type Notion's schema does not report. "text" is deliberately not
// Notion's own key: a formula's text filter is keyed "string" and a rollup's
// "rich_text", and asking a model to know which is which buys nothing.
const (
	valueTypeText     = "text"
	valueTypeNumber   = "number"
	valueTypeCheckbox = "checkbox"
	valueTypeDate     = "date"
)

var (
	valueTypeNames        = []string{valueTypeText, valueTypeNumber, valueTypeCheckbox, valueTypeDate}
	rollupAggregations    = []string{"any", "every", "none"}
	emptinessOperators    = []string{"is_empty", "is_not_empty"}
	relativeDateOperators = []string{"past_week", "past_month", "past_year", "this_week", "next_week", "next_month", "next_year"}
)

// conditionSpec is one condition as the agent writes it.
type conditionSpec struct {
	// Property is the property's display name, as listed in property_schema.
	Property string
	// Operator is one of the operators that property's type accepts.
	Operator string
	// Value is what to compare against, written as text. Ignored by the
	// operators that take no value.
	Value string
	// ValueType is required for a formula or rollup property: what it
	// evaluates to. One of valueTypeNames.
	ValueType string
	// Aggregation is how many of a rollup's rolled-up values must match. One of
	// rollupAggregations, and meaningful for a rollup over a list only.
	Aggregation string
}

// groupSpec is a nested run of conditions. Notion allows one level of nesting,
// so a group holds plain conditions and nothing deeper.
type groupSpec struct {
	Operator   string
	Conditions []conditionSpec
}

// filterSpec is the whole condition tree as the agent writes it. It is flat by
// design: Notion nests two levels deep and no more, so "the top-level operator,
// its conditions, and its groups" expresses exactly what is allowed and cannot
// express anything that is not.
type filterSpec struct {
	Operator   string
	Conditions []conditionSpec
	Groups     []groupSpec
}

// sortSpec is one ordering key.
type sortSpec struct {
	Property  string
	Timestamp string
	Direction string
}

// searchableAs says how a keyword is matched against one property.
//
// Notion has no substring match for a choice property, so a keyword lands on
// select and status as an exact choice name and on multi_select as "holds this
// choice". Every other type is refused rather than matched loosely: a keyword
// compared against a date or a number is a query that silently returns nothing.
func searchableAs(prop PropertySchema) (key, operator string, ok bool) {
	switch prop.Type {
	case propTypeTitle, propTypeRichText, propTypeURL, propTypeEmail, propTypePhoneNumber:
		return propTypeRichText, operatorContains, true
	case propTypeMultiSelect:
		return propTypeMultiSelect, operatorContains, true
	case propTypeSelect:
		return propTypeSelect, operatorEquals, true
	case propTypeStatus:
		return propTypeStatus, operatorEquals, true
	}
	return "", "", false
}

// findTitleProperty finds the data source's title column, which is what a keyword
// search falls back to when the caller names no target.
func findTitleProperty(ds *DataSource) (PropertySchema, bool) {
	for _, p := range ds.Properties {
		if p.Type == propTypeTitle {
			return p, true
		}
	}
	return PropertySchema{}, false
}

// resolveSearchTargets decides which properties the keywords are matched
// against: the ones the caller named, or the title column when it named none.
func resolveSearchTargets(ds *DataSource, names []string) ([]PropertySchema, error) {
	if len(names) == 0 {
		title, ok := findTitleProperty(ds)
		if !ok {
			return nil, rejectf("this data source has no title column, so a keyword search has nothing to match by default; name the properties to search in search_properties")
		}
		return []PropertySchema{title}, nil
	}

	targets, err := resolveProperties(ds, names)
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		if _, _, ok := searchableAs(target); !ok {
			return nil, rejectf("property %q has type %q and cannot be matched by keyword; name a text, select, status or multi_select property in search_properties", target.Name, target.Type)
		}
	}
	return targets, nil
}

// buildKeywordFilter turns the keywords into one filter part per keyword: a
// single condition when there is one target property, an "or" over the targets
// when there are several.
//
// The parts are combined with "and" by the caller, so a row has to match every
// keyword somewhere among the targets. That shape — and with an or inside — is
// exactly Notion's two-level limit, which is why a caller's own or-group cannot
// be combined with a keyword search (see buildFilter).
func buildKeywordFilter(targets []PropertySchema, keywords []string) ([]map[string]any, error) {
	parts := make([]map[string]any, 0, len(keywords))
	for _, keyword := range keywords {
		leaves := make([]map[string]any, 0, len(targets))
		for _, target := range targets {
			key, operator, ok := searchableAs(target)
			if !ok {
				return nil, rejectf("property %q has type %q and cannot be matched by keyword", target.Name, target.Type)
			}
			leaves = append(leaves, map[string]any{
				"property": target.ID,
				key:        map[string]any{operator: keyword},
			})
		}
		if len(leaves) == 1 {
			parts = append(parts, leaves[0])
			continue
		}
		parts = append(parts, map[string]any{operatorOr: leaves})
	}
	return parts, nil
}

// buildFilter combines the keyword search and the caller's own conditions into
// one Notion filter, or nil when neither narrows anything.
//
// The one combination it refuses is a keyword search alongside a caller filter
// that is an "or" AND carries groups: the keyword parts already occupy the top
// "and" and the groups occupy the level below, so wrapping that or would be a
// third level and Notion allows two.
func buildFilter(ds *DataSource, keywords []string, targets []PropertySchema, spec *filterSpec) (map[string]any, error) {
	keywordParts, err := buildKeywordFilter(targets, keywords)
	if err != nil {
		return nil, err
	}

	userParts, userOperator, userHasGroups, err := buildUserFilter(ds, spec)
	if err != nil {
		return nil, err
	}

	switch {
	case len(keywordParts) == 0 && len(userParts) == 0:
		return nil, nil

	case len(keywordParts) == 0:
		if len(userParts) == 1 {
			return userParts[0], nil
		}
		return map[string]any{userOperator: userParts}, nil

	case len(userParts) == 0:
		if len(keywordParts) == 1 {
			return keywordParts[0], nil
		}
		return map[string]any{operatorAnd: keywordParts}, nil

	case userOperator == operatorAnd:
		// Both sides are already an "and" of parts, so they merge into one
		// array without adding a level.
		combined := make([]map[string]any, 0, len(keywordParts)+len(userParts))
		combined = append(combined, keywordParts...)
		combined = append(combined, userParts...)
		return map[string]any{operatorAnd: combined}, nil

	case userHasGroups:
		return nil, rejectf("query cannot be combined with a filter whose operator is %q and which also has groups, because Notion nests only two levels deep; write the keywords as conditions inside filter instead", operatorOr)

	default:
		combined := make([]map[string]any, 0, len(keywordParts)+1)
		combined = append(combined, keywordParts...)
		combined = append(combined, map[string]any{operatorOr: userParts})
		return map[string]any{operatorAnd: combined}, nil
	}
}

// buildUserFilter renders the caller's own conditions as the parts of one
// compound filter, and reports the operator they combine under and whether any
// of them is itself a group.
func buildUserFilter(ds *DataSource, spec *filterSpec) (parts []map[string]any, operator string, hasGroups bool, err error) {
	if spec == nil {
		return nil, operatorAnd, false, nil
	}

	operator = spec.Operator
	if operator == "" {
		operator = operatorAnd
	}
	if operator != operatorAnd && operator != operatorOr {
		return nil, "", false, rejectf("filter.operator must be %q or %q", operatorAnd, operatorOr)
	}

	total := len(spec.Conditions)
	for _, group := range spec.Groups {
		total += len(group.Conditions)
	}
	if total > filterConditionsMax {
		return nil, "", false, rejectf("filter holds %d conditions; at most %d are accepted", total, filterConditionsMax)
	}
	if len(spec.Groups) > filterGroupsMax {
		return nil, "", false, rejectf("filter holds %d groups; at most %d are accepted", len(spec.Groups), filterGroupsMax)
	}

	parts = make([]map[string]any, 0, len(spec.Conditions)+len(spec.Groups))
	for _, condition := range spec.Conditions {
		leaf, err := buildCondition(ds, condition)
		if err != nil {
			return nil, "", false, err
		}
		parts = append(parts, leaf)
	}

	for i, group := range spec.Groups {
		groupOperator := group.Operator
		if groupOperator != operatorAnd && groupOperator != operatorOr {
			return nil, "", false, rejectf("filter.groups[%d].operator must be %q or %q", i, operatorAnd, operatorOr)
		}
		if len(group.Conditions) == 0 {
			return nil, "", false, rejectf("filter.groups[%d] holds no conditions", i)
		}
		leaves := make([]map[string]any, 0, len(group.Conditions))
		for _, condition := range group.Conditions {
			leaf, err := buildCondition(ds, condition)
			if err != nil {
				return nil, "", false, err
			}
			leaves = append(leaves, leaf)
		}
		parts = append(parts, map[string]any{groupOperator: leaves})
		hasGroups = true
	}

	return parts, operator, hasGroups, nil
}

// buildCondition renders one condition against the property it names.
func buildCondition(ds *DataSource, c conditionSpec) (map[string]any, error) {
	prop, err := resolveProperty(ds, c.Property)
	if err != nil {
		return nil, err
	}

	switch prop.Type {
	case propTypeFormula:
		kind, err := declaredKind(prop, c)
		if err != nil {
			return nil, err
		}
		body, err := buildConditionBody(prop, c, kind)
		if err != nil {
			return nil, err
		}
		return map[string]any{"property": prop.ID, propTypeFormula: map[string]any{kind.key: body}}, nil

	case propTypeRollup:
		return buildRollupCondition(prop, c)
	}

	kind, ok := conditionKindFor(prop.Type)
	if !ok {
		return nil, rejectf("property %q has type %q, which Notion cannot filter on", prop.Name, prop.Type)
	}
	body, err := buildConditionBody(prop, c, kind)
	if err != nil {
		return nil, err
	}
	if kind.timestamp != "" {
		// created_time and last_edited_time take Notion's top-level timestamp
		// filter, which names no property at all.
		return map[string]any{"timestamp": kind.timestamp, kind.key: body}, nil
	}
	return map[string]any{"property": prop.ID, kind.key: body}, nil
}

// buildRollupCondition renders a rollup's condition. Notion nests it twice: a
// rollup over a list takes an aggregation around the inner condition, while one
// that computes a number or a date takes the condition directly.
func buildRollupCondition(prop PropertySchema, c conditionSpec) (map[string]any, error) {
	kind, err := declaredKind(prop, c)
	if err != nil {
		return nil, err
	}
	body, err := buildConditionBody(prop, c, kind)
	if err != nil {
		return nil, err
	}

	if c.Aggregation != "" {
		if !containsString(rollupAggregations, c.Aggregation) {
			return nil, rejectf("aggregation %q on property %q is not one of: %s", c.Aggregation, prop.Name, strings.Join(rollupAggregations, ", "))
		}
		return map[string]any{"property": prop.ID, propTypeRollup: map[string]any{
			c.Aggregation: map[string]any{kind.key: body},
		}}, nil
	}

	if c.ValueType != valueTypeNumber && c.ValueType != valueTypeDate {
		return nil, rejectf("rollup property %q needs an aggregation (%s) unless value_type is %q or %q, because Notion filters a rollup over a list through one of those",
			prop.Name, strings.Join(rollupAggregations, " / "), valueTypeNumber, valueTypeDate)
	}
	return map[string]any{"property": prop.ID, propTypeRollup: map[string]any{kind.key: body}}, nil
}

// declaredKind resolves the result type the caller declared for a formula or
// rollup property.
func declaredKind(prop PropertySchema, c conditionSpec) (conditionKind, error) {
	if c.ValueType == "" {
		return conditionKind{}, rejectf("property %q is a %s, and Notion's schema does not report what it evaluates to, so value_type is required; set it to one of: %s",
			prop.Name, prop.Type, strings.Join(valueTypeNames, ", "))
	}
	kind, ok := nestedConditionKind(prop.Type, c.ValueType)
	if !ok {
		return conditionKind{}, rejectf("value_type %q on property %q is not one of: %s", c.ValueType, prop.Name, strings.Join(valueTypeNames, ", "))
	}
	return kind, nil
}

// nestedConditionKind maps a declared result type onto the condition shape its
// inner filter takes. The text key differs between the two outer types: Notion
// keys a formula's text filter "string" and a rollup's "rich_text".
func nestedConditionKind(outerType, valueType string) (conditionKind, bool) {
	switch valueType {
	case valueTypeText:
		key := propTypeRichText
		if outerType == propTypeFormula {
			key = formulaKindString
		}
		return conditionKind{key: key, operators: textOperators, value: valueString}, true
	case valueTypeNumber:
		return conditionKind{key: propTypeNumber, operators: numberOperators, value: valueNumber}, true
	case valueTypeCheckbox:
		return conditionKind{key: propTypeCheckbox, operators: checkboxOperators, value: valueBool}, true
	case valueTypeDate:
		return conditionKind{key: propTypeDate, operators: dateOperators, value: valueDate}, true
	}
	return conditionKind{}, false
}

// buildConditionBody renders the innermost object: the operator and its value.
func buildConditionBody(prop PropertySchema, c conditionSpec, kind conditionKind) (map[string]any, error) {
	if !containsString(kind.operators, c.Operator) {
		return nil, rejectf("operator %q cannot be used on property %q of type %q; that accepts: %s",
			c.Operator, prop.Name, prop.Type, strings.Join(kind.operators, ", "))
	}

	// Notion writes an emptiness check as a boolean and a relative date as an
	// empty object; neither takes the caller's value.
	if containsString(emptinessOperators, c.Operator) {
		return map[string]any{c.Operator: true}, nil
	}
	if containsString(relativeDateOperators, c.Operator) {
		return map[string]any{c.Operator: map[string]any{}}, nil
	}

	if c.Value == "" {
		return nil, rejectf("operator %q on property %q needs a value", c.Operator, prop.Name)
	}
	value, err := coerceValue(kind.value, prop, c.Value)
	if err != nil {
		return nil, err
	}
	return map[string]any{c.Operator: value}, nil
}

// coerceValue turns the value, which arrives as text, into the JSON type the
// property holds. A value that cannot be read that way is a rejection: sending
// "forty-two" where Notion expects a number is a 400 whose message does not say
// which argument was wrong.
func coerceValue(kind valueKind, prop PropertySchema, raw string) (any, error) {
	switch kind {
	case valueString:
		return raw, nil
	case valueNumber:
		number, err := strconv.ParseFloat(raw, 64)
		// ParseFloat accepts "NaN", "Inf" and "-Inf". None of them can be
		// encoded as JSON, so letting one through turns a repairable argument
		// into a request that fails to serialise — reported as an internal
		// error rather than as something the caller can restate.
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, rejectf("property %q holds a number, so value %q must be written as digits, for example \"42\"", prop.Name, raw)
		}
		return number, nil
	case valueInteger:
		number, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, rejectf("property %q holds a whole-number id, so value %q must be written as digits with no decimal point, for example \"42\"", prop.Name, raw)
		}
		return number, nil
	case valueBool:
		flag, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, rejectf("property %q holds a checkbox, so value %q must be \"true\" or \"false\"", prop.Name, raw)
		}
		return flag, nil
	case valueDate:
		if !isISODate(raw) {
			return nil, rejectf("property %q holds a date, so value %q must be ISO 8601, for example \"2026-01-31\" or \"2026-01-31T09:00:00Z\"", prop.Name, raw)
		}
		return raw, nil
	}
	return nil, rejectf("property %q of type %q takes no value; use is_empty or is_not_empty", prop.Name, prop.Type)
}

func isISODate(raw string) bool {
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if _, err := time.Parse(layout, raw); err == nil {
			return true
		}
	}
	return false
}

// buildSorts renders the ordering keys.
func buildSorts(ds *DataSource, specs []sortSpec) ([]map[string]any, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	if len(specs) > sortsMax {
		return nil, rejectf("sorts holds %d entries; at most %d are accepted", len(specs), sortsMax)
	}

	out := make([]map[string]any, 0, len(specs))
	for i, spec := range specs {
		// Notion's own default is ascending, so an unstated direction stays
		// ascending rather than being guessed at.
		direction := spec.Direction
		if direction == "" {
			direction = directionAscending
		}
		if direction != directionAscending && direction != directionDescending {
			return nil, rejectf("sorts[%d].direction must be %q or %q", i, directionAscending, directionDescending)
		}

		switch {
		case spec.Property != "" && spec.Timestamp != "":
			return nil, rejectf("sorts[%d] sets both property and timestamp; set exactly one", i)
		case spec.Property != "":
			prop, err := resolveProperty(ds, spec.Property)
			if err != nil {
				return nil, err
			}
			out = append(out, map[string]any{"property": prop.ID, "direction": direction})
		case spec.Timestamp != "":
			if spec.Timestamp != propTypeCreatedTime && spec.Timestamp != propTypeLastEditedTime {
				return nil, rejectf("sorts[%d].timestamp must be %q or %q", i, propTypeCreatedTime, propTypeLastEditedTime)
			}
			out = append(out, map[string]any{"timestamp": spec.Timestamp, "direction": direction})
		default:
			return nil, rejectf("sorts[%d] sets neither property nor timestamp; set exactly one", i)
		}
	}
	return out, nil
}

// parseSearchQuery splits the keyword argument on whitespace. strings.Fields
// counts the ideographic space as whitespace, so a query typed on a Japanese
// keyboard splits the same way as one typed on an English keyboard.
func parseSearchQuery(args map[string]any) ([]string, error) {
	raw, ok := args["query"]
	if !ok || raw == nil {
		return nil, nil
	}
	text, ok := raw.(string)
	if !ok {
		return nil, rejectf("query must be text: the keywords a row has to contain, separated by spaces")
	}
	keywords := strings.Fields(text)
	if len(keywords) > queryKeywordsMax {
		return nil, rejectf("query holds %d keywords; at most %d are accepted", len(keywords), queryKeywordsMax)
	}
	return keywords, nil
}

// parseFilterArgs reads the filter argument. A shape that is not the documented
// one is a rejection rather than an error: it is the model's mistake, and it can
// restate the call.
func parseFilterArgs(args map[string]any) (*filterSpec, error) {
	raw, ok := args["filter"]
	if !ok || raw == nil {
		return nil, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, rejectf("filter must be an object holding operator, conditions and groups")
	}

	spec := &filterSpec{}
	operator, err := stringField(obj, "filter", "operator")
	if err != nil {
		return nil, err
	}
	spec.Operator = operator

	conditions, err := parseConditions(obj["conditions"], "filter.conditions")
	if err != nil {
		return nil, err
	}
	spec.Conditions = conditions

	if groups, ok := obj["groups"]; ok && groups != nil {
		items, ok := groups.([]any)
		if !ok {
			return nil, rejectf("filter.groups must be an array of objects holding operator and conditions")
		}
		for i, item := range items {
			groupObj, ok := item.(map[string]any)
			if !ok {
				return nil, rejectf("filter.groups[%d] must be an object holding operator and conditions", i)
			}
			label := fmt.Sprintf("filter.groups[%d]", i)
			groupOperator, err := stringField(groupObj, label, "operator")
			if err != nil {
				return nil, err
			}
			groupConditions, err := parseConditions(groupObj["conditions"], label+".conditions")
			if err != nil {
				return nil, err
			}
			spec.Groups = append(spec.Groups, groupSpec{Operator: groupOperator, Conditions: groupConditions})
		}
	}
	return spec, nil
}

func parseConditions(raw any, label string) ([]conditionSpec, error) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, rejectf("%s must be an array of condition objects", label)
	}

	out := make([]conditionSpec, 0, len(items))
	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, rejectf("%s[%d] must be an object holding property, operator and value", label, i)
		}
		entry := fmt.Sprintf("%s[%d]", label, i)

		var spec conditionSpec
		for _, field := range []struct {
			key    string
			target *string
		}{
			{"property", &spec.Property},
			{"operator", &spec.Operator},
			{"value", &spec.Value},
			{"value_type", &spec.ValueType},
			{"aggregation", &spec.Aggregation},
		} {
			value, err := stringField(obj, entry, field.key)
			if err != nil {
				return nil, err
			}
			*field.target = value
		}
		out = append(out, spec)
	}
	return out, nil
}

func parseSortArgs(args map[string]any) ([]sortSpec, error) {
	raw, ok := args["sorts"]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, rejectf("sorts must be an array of objects holding property or timestamp, and direction")
	}

	out := make([]sortSpec, 0, len(items))
	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, rejectf("sorts[%d] must be an object holding property or timestamp, and direction", i)
		}
		entry := fmt.Sprintf("sorts[%d]", i)

		var spec sortSpec
		for _, field := range []struct {
			key    string
			target *string
		}{
			{"property", &spec.Property},
			{"timestamp", &spec.Timestamp},
			{"direction", &spec.Direction},
		} {
			value, err := stringField(obj, entry, field.key)
			if err != nil {
				return nil, err
			}
			*field.target = value
		}
		out = append(out, spec)
	}
	return out, nil
}

// stringField reads one text field of an argument object. A number or a boolean
// is accepted and rendered as text: the arguments declare every value as text,
// but a model writing `"value": 42` for a number column has made no mistake
// worth a round trip.
func stringField(obj map[string]any, label, key string) (string, error) {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return "", nil
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(value), nil
	}
	return "", rejectf("%s.%s must be text", label, key)
}

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
	// valueInteger is the unique_id column's kind. Notion's unique_id is a
	// counter, and its filter conditions take an integer — a fraction sent
	// there is a 400 whose message does not say which argument was wrong.
	valueInteger
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
		return conditionKind{key: propTypeUniqueID, operators: uniqueIDOperators, value: valueInteger}, true
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
