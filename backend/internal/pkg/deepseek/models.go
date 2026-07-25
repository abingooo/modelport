package deepseek

const (
	DefaultBaseURL   = "https://api.deepseek.com"
	DefaultTestModel = "deepseek-chat"
)

var defaultModelIDs = []string{
	"deepseek-chat",
	"deepseek-reasoner",
}

func DefaultModelIDs() []string {
	models := make([]string, len(defaultModelIDs))
	copy(models, defaultModelIDs)
	return models
}
