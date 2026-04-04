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
