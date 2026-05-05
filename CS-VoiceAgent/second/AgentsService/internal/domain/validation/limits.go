package validation

// Лимиты вынесены в отдельный файл, чтобы:
// - не было \"магических чисел\" по коду,
// - лимиты легко было сравнить с ТЗ.
const (
	MaxLenName255               = 255
	MaxLenVarName128            = 128
	MaxLenVarDescription2000    = 2000
	MaxLenEnumChoice255         = 255
	MaxLenGlobalPrompt50000     = 50000
	MaxLenInstructionText50000  = 50000
	MaxLenEdgePrompt10000       = 10000
	MaxLenCondition10000        = 10000
	MaxLenTranscriptItem10000   = 10000

	MaxNodesPerWorkflow1000 = 1000
	MaxEdgesPerNode50       = 50
	MaxVarsPerNode100       = 100
	MaxEnumChoices100       = 100
	MaxExamplesPerNode50    = 50
	MaxTranscriptItems100   = 100

	// MaxWorkflowJSONBytes — лимит на сериализованный ConversationFlow JSON (по ТЗ: 8MB).
	MaxWorkflowJSONBytes = 8 * 1024 * 1024
)

