package toolsim

import notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"

// NotionPropertiesFromForTest exposes the conversion from the simulator's reply
// to a Notion column schema. Driving it through GetDataSource would need an LLM
// completer for what is a pure parse of one string.
func NotionPropertiesFromForTest(text string) []notiontool.PropertySchema {
	return notionPropertiesFrom(text)
}
