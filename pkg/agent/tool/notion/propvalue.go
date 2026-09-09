package notiontool

import (
	"encoding/json"
	"strconv"
	"strings"
)

// propertyValueMaxLen bounds one rendered property value, in runes. It is
// generous because a summary property is exactly the kind of column an agent
// asks for in order to quote it as evidence, and a value cut at 100 characters
// would be useless for that.
const propertyValueMaxLen = 500

// propertyListMaxItems bounds how many entries of a list-valued property are
// joined. A relation column can hold hundreds of ids.
const propertyListMaxItems = 10

// The result kinds a formula and a rollup report. Notion keys a formula's text
// result under "string" and its boolean under "boolean", neither of which
// matches the property type of the same value elsewhere; a rollup over a list
// reports "array".
const (
	formulaKindString  = "string"
	formulaKindBoolean = "boolean"
	rollupKindArray    = "array"
)

// propertyValue is one Notion property value, narrowed to the fields the text
// rendering needs. Every field is a pointer or a slice so a JSON null is
// distinguishable from an absent key, and unknown fields are ignored: a
// property type Notion adds later must not fail the row (see
// renderPropertyValue).
type propertyValue struct {
	Type string `json:"type"`

	Title          []richText     `json:"title"`
	RichText       []richText     `json:"rich_text"`
	Number         *float64       `json:"number"`
	Select         *namedChoice   `json:"select"`
	Status         *namedChoice   `json:"status"`
	MultiSelect    []namedChoice  `json:"multi_select"`
	Date           *dateValue     `json:"date"`
	Checkbox       *bool          `json:"checkbox"`
	URL            *string        `json:"url"`
	Email          *string        `json:"email"`
	PhoneNumber    *string        `json:"phone_number"`
	People         []userValue    `json:"people"`
	Files          []fileValue    `json:"files"`
	Relation       []idValue      `json:"relation"`
	CreatedTime    *string        `json:"created_time"`
	LastEditedTime *string        `json:"last_edited_time"`
	CreatedBy      *userValue     `json:"created_by"`
	LastEditedBy   *userValue     `json:"last_edited_by"`
	UniqueID       *uniqueIDValue `json:"unique_id"`
	Formula        *formulaValue  `json:"formula"`
	Rollup         *rollupValue   `json:"rollup"`
}

type namedChoice struct {
	Name string `json:"name"`
}

type dateValue struct {
	Start *string `json:"start"`
	End   *string `json:"end"`
}

// userValue is a Notion user narrowed to what a row can display. Name is empty
// when the integration's user capability is set to "no user information", in
// which case the id is shown instead.
type userValue struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type fileValue struct {
	Name string `json:"name"`
}

type idValue struct {
	ID string `json:"id"`
}

type uniqueIDValue struct {
	Prefix *string  `json:"prefix"`
	Number *float64 `json:"number"`
}

// formulaValue holds a formula's result. Notion keys a formula's text result
// under "string" (its "text" key is deprecated), which is NOT the "rich_text"
// key the same value takes everywhere else.
type formulaValue struct {
	Type    string     `json:"type"`
	String  *string    `json:"string"`
	Number  *float64   `json:"number"`
	Boolean *bool      `json:"boolean"`
	Date    *dateValue `json:"date"`
}

// rollupValue holds a rollup's result. An array rollup's entries are property
// values in their own right, so they stay raw and are rendered recursively.
type rollupValue struct {
	Type   string            `json:"type"`
	Number *float64          `json:"number"`
	Date   *dateValue        `json:"date"`
	Array  []json.RawMessage `json:"array"`
}

// renderPropertyValue renders one property value as a single line of text.
//
// The second return value says whether the value could be rendered at all. It
// is false for a property type this package does not model and for a payload
// that does not decode, and the caller drops that one column rather than
// failing the row: Notion keeps adding property types (a `place` property in
// production is what made the search endpoint's own decoder narrow), and a row
// is still worth returning without one of its columns.
//
// A value that is present but null renders as the empty string with ok true, so
// "this row has no value here" stays distinguishable from "this column does not
// exist".
func renderPropertyValue(raw json.RawMessage) (string, bool) {
	var value propertyValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}

	text, ok := renderValue(&value)
	if !ok {
		return "", false
	}
	return truncate(text, propertyValueMaxLen), true
}

func renderValue(v *propertyValue) (string, bool) {
	switch v.Type {
	case propTypeTitle:
		return plainText(v.Title), true
	case propTypeRichText:
		return plainText(v.RichText), true
	case propTypeNumber:
		return formatNumber(v.Number), true
	case propTypeSelect:
		return choiceName(v.Select), true
	case propTypeStatus:
		return choiceName(v.Status), true
	case propTypeMultiSelect:
		return joinChoices(v.MultiSelect), true
	case propTypeDate:
		return formatDate(v.Date), true
	case propTypeCheckbox:
		return formatBool(v.Checkbox), true
	case propTypeURL:
		return derefString(v.URL), true
	case propTypeEmail:
		return derefString(v.Email), true
	case propTypePhoneNumber:
		return derefString(v.PhoneNumber), true
	case propTypePeople:
		return joinUsers(v.People), true
	case propTypeFiles:
		return joinFiles(v.Files), true
	case propTypeRelation:
		return joinIDs(v.Relation), true
	case propTypeCreatedTime:
		return derefString(v.CreatedTime), true
	case propTypeLastEditedTime:
		return derefString(v.LastEditedTime), true
	case propTypeCreatedBy:
		return userLabel(v.CreatedBy), true
	case propTypeLastEditedBy:
		return userLabel(v.LastEditedBy), true
	case propTypeUniqueID:
		return formatUniqueID(v.UniqueID), true
	case propTypeFormula:
		return renderFormula(v.Formula)
	case propTypeRollup:
		return renderRollup(v.Rollup)
	}
	return "", false
}

func renderFormula(f *formulaValue) (string, bool) {
	if f == nil {
		return "", true
	}
	switch f.Type {
	case formulaKindString:
		return derefString(f.String), true
	case propTypeNumber:
		return formatNumber(f.Number), true
	case formulaKindBoolean:
		return formatBool(f.Boolean), true
	case propTypeDate:
		return formatDate(f.Date), true
	}
	return "", false
}

func renderRollup(r *rollupValue) (string, bool) {
	if r == nil {
		return "", true
	}
	switch r.Type {
	case propTypeNumber:
		return formatNumber(r.Number), true
	case propTypeDate:
		return formatDate(r.Date), true
	case rollupKindArray:
		parts := make([]string, 0, len(r.Array))
		for _, item := range r.Array {
			text, ok := renderPropertyValue(item)
			if !ok || text == "" {
				continue
			}
			parts = append(parts, text)
		}
		return joinBounded(parts), true
	}
	return "", false
}

// joinBounded joins at most propertyListMaxItems entries and marks the cut, so
// a shortened list is distinguishable from a complete one.
func joinBounded(parts []string) string {
	if len(parts) <= propertyListMaxItems {
		return strings.Join(parts, ", ")
	}
	return strings.Join(parts[:propertyListMaxItems], ", ") + ", ..."
}

func choiceName(c *namedChoice) string {
	if c == nil {
		return ""
	}
	return c.Name
}

func joinChoices(choices []namedChoice) string {
	parts := make([]string, 0, len(choices))
	for _, c := range choices {
		if c.Name == "" {
			continue
		}
		parts = append(parts, c.Name)
	}
	return joinBounded(parts)
}

func joinUsers(users []userValue) string {
	parts := make([]string, 0, len(users))
	for i := range users {
		if label := userLabel(&users[i]); label != "" {
			parts = append(parts, label)
		}
	}
	return joinBounded(parts)
}

// userLabel prefers the user's name and falls back to the id. The fallback is
// the normal outcome when the integration may not read user information.
func userLabel(u *userValue) string {
	if u == nil {
		return ""
	}
	if u.Name != "" {
		return u.Name
	}
	return u.ID
}

func joinFiles(files []fileValue) string {
	parts := make([]string, 0, len(files))
	for _, f := range files {
		if f.Name == "" {
			continue
		}
		parts = append(parts, f.Name)
	}
	return joinBounded(parts)
}

func joinIDs(ids []idValue) string {
	parts := make([]string, 0, len(ids))
	for _, v := range ids {
		if v.ID == "" {
			continue
		}
		parts = append(parts, v.ID)
	}
	return joinBounded(parts)
}

func formatDate(d *dateValue) string {
	if d == nil {
		return ""
	}
	start := derefString(d.Start)
	end := derefString(d.End)
	if end == "" {
		return start
	}
	// An ASCII arrow rather than a dash: an ISO date already contains dashes.
	return start + " -> " + end
}

// formatNumber prints the number the way it was written, without a forced
// decimal point: 'f' with precision -1 gives "42" for 42 and "1.5" for 1.5.
func formatNumber(n *float64) string {
	if n == nil {
		return ""
	}
	return strconv.FormatFloat(*n, 'f', -1, 64)
}

func formatBool(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

func formatUniqueID(u *uniqueIDValue) string {
	if u == nil || u.Number == nil {
		return ""
	}
	number := formatNumber(u.Number)
	if prefix := derefString(u.Prefix); prefix != "" {
		return prefix + "-" + number
	}
	return number
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
