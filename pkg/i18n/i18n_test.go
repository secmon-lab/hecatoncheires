package i18n_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
)

func TestT(t *testing.T) {
	i18n.Init(i18n.LangEN)

	t.Run("returns English translation", func(t *testing.T) {
		ctx := i18n.ContextWithLang(context.Background(), i18n.LangEN)
		result := i18n.T(ctx, i18n.MsgModalCreateCaseTitle)
		gt.Value(t, result).Equal("Create Case")
	})

	t.Run("returns Japanese translation", func(t *testing.T) {
		ctx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		result := i18n.T(ctx, i18n.MsgModalCreateCaseTitle)
		gt.Value(t, result).Equal("ケース作成")
	})

	t.Run("formats with args", func(t *testing.T) {
		ctx := i18n.ContextWithLang(context.Background(), i18n.LangEN)
		result := i18n.T(ctx, i18n.MsgCaseCreated, 42, "Test Case")
		gt.Value(t, result).Equal("Case #42 *Test Case* has been created.")
	})

	t.Run("formats Japanese with args", func(t *testing.T) {
		ctx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		result := i18n.T(ctx, i18n.MsgCaseCreated, 42, "テストケース")
		gt.Value(t, result).Equal("ケース #42 *テストケース* が作成されました。")
	})

	t.Run("resolves notification fallback keys", func(t *testing.T) {
		enCtx := i18n.ContextWithLang(context.Background(), i18n.LangEN)
		jaCtx := i18n.ContextWithLang(context.Background(), i18n.LangJA)

		gt.Value(t, i18n.T(enCtx, i18n.MsgMentionCanceledFallback)).Equal("Case draft canceled")
		gt.Value(t, i18n.T(jaCtx, i18n.MsgMentionCanceledFallback)).Equal("ケースの下書きをキャンセルしました")

		gt.Value(t, i18n.T(enCtx, i18n.MsgMentionQuestionFallback)).Equal("We need a bit more info to draft this case.")
		gt.Value(t, i18n.T(jaCtx, i18n.MsgMentionQuestionFallback)).Equal("ケースの下書きにはもう少し情報が必要です。")

		gt.Value(t, i18n.T(enCtx, i18n.MsgMentionPreviewFallbackWithTitle, "Broken auth")).Equal("Case draft: Broken auth")
		gt.Value(t, i18n.T(jaCtx, i18n.MsgMentionPreviewFallbackWithTitle, "Broken auth")).Equal("ケース下書き: Broken auth")

		gt.Value(t, i18n.T(enCtx, i18n.MsgCaseCreatedFallback, 42, "Broken auth")).Equal("Created case #42: Broken auth")
		gt.Value(t, i18n.T(jaCtx, i18n.MsgCaseCreatedFallback, 42, "Broken auth")).Equal("ケース #42 を作成しました: Broken auth")

		gt.Value(t, i18n.T(enCtx, i18n.MsgThreadCaseQuestionFallback)).Equal("We need a bit more info to create this case.")
		gt.Value(t, i18n.T(jaCtx, i18n.MsgThreadCaseQuestionFallback)).Equal("ケースの作成にはもう少し情報が必要です。")
	})

	t.Run("falls back to default lang for no lang in context", func(t *testing.T) {
		result := i18n.T(context.Background(), i18n.MsgModalCreateCaseTitle)
		gt.Value(t, result).Equal("Create Case")
	})

	t.Run("returns default lang with Japanese default", func(t *testing.T) {
		i18n.Init(i18n.LangJA)
		defer i18n.Init(i18n.LangEN) // restore
		result := i18n.T(context.Background(), i18n.MsgModalCreateCaseTitle)
		gt.Value(t, result).Equal("ケース作成")
	})
}

func TestDefaultLang(t *testing.T) {
	t.Run("returns configured default", func(t *testing.T) {
		i18n.Init(i18n.LangJA)
		defer i18n.Init(i18n.LangEN)
		gt.Value(t, i18n.DefaultLang()).Equal(i18n.LangJA)
	})
}

// LanguageLabel is what the agent hosts put in planexec's Input.LanguageLabel, and
// the runtime renders it into the "write user-facing copy in X" directive. The
// label must therefore be the language's English NAME, not its code: a prompt
// saying "write in ja" is not the instruction the directive intends. An empty
// return is meaningful too — it omits the directive rather than emitting an empty
// one.
func TestLanguageLabel(t *testing.T) {
	t.Run("names the language in the context", func(t *testing.T) {
		i18n.Init(i18n.LangEN)
		gt.Value(t, i18n.LanguageLabel(i18n.ContextWithLang(context.Background(), i18n.LangJA))).
			Equal("Japanese")
		gt.Value(t, i18n.LanguageLabel(i18n.ContextWithLang(context.Background(), i18n.LangEN))).
			Equal("English")
	})

	t.Run("falls back to the configured default when the context names none", func(t *testing.T) {
		i18n.Init(i18n.LangJA)
		defer i18n.Init(i18n.LangEN)
		gt.Value(t, i18n.LanguageLabel(context.Background())).Equal("Japanese")
	})

	t.Run("returns empty for an unsupported language, which omits the directive", func(t *testing.T) {
		i18n.Init(i18n.LangEN)
		gt.Value(t, i18n.LanguageLabel(i18n.ContextWithLang(context.Background(), i18n.Lang("fr")))).
			Equal("")
	})
}

func TestDetectLang(t *testing.T) {
	tests := []struct {
		locale string
		want   i18n.Lang
	}{
		{"ja-JP", i18n.LangJA},
		{"ja", i18n.LangJA},
		{"en-US", i18n.LangEN},
		{"en", i18n.LangEN},
		{"fr-FR", i18n.Lang("")},
		{"", i18n.Lang("")},
	}

	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			got := i18n.DetectLang(tt.locale)
			gt.Value(t, got).Equal(tt.want)
		})
	}
}

func TestParseLang(t *testing.T) {
	t.Run("parses en", func(t *testing.T) {
		lang, err := i18n.ParseLang("en")
		gt.NoError(t, err)
		gt.Value(t, lang).Equal(i18n.LangEN)
	})

	t.Run("parses ja", func(t *testing.T) {
		lang, err := i18n.ParseLang("ja")
		gt.NoError(t, err)
		gt.Value(t, lang).Equal(i18n.LangJA)
	})

	t.Run("rejects unsupported language", func(t *testing.T) {
		_, err := i18n.ParseLang("fr")
		gt.Error(t, err)
	})
}
