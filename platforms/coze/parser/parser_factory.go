package parser

import (
	"fmt"
	"github.com/iflytek/agentbridge/internal/models"
)

// ParserFactory creates Coze node parsers.
type ParserFactory struct {
	parsers map[string]func(*models.VariableReferenceSystem) NodeParser
}

func NewParserFactory() *ParserFactory {
	factory := &ParserFactory{
		parsers: make(map[string]func(*models.VariableReferenceSystem) NodeParser),
	}

	// Register basic node parsers (numeric IDs)
	factory.Register("1", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewStartNodeParser(vrs)
	})
	factory.Register("2", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewEndNodeParser(vrs)
	})

	// Register LLM node parser (Phase 2)
	factory.Register("3", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewLLMNodeParser(vrs)
	})
	// factory.Register("4", func(vrs *models.VariableReferenceSystem) NodeParser {
	//     return NewConditionNodeParser(vrs)
	// })
	factory.Register("5", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewCodeNodeParser(vrs)
	})

	// Register Classifier node parser (Phase 6)
	factory.Register("22", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewClassifierNodeParser(vrs)
	})

	// Register Iteration node parser (Phase 7)
	factory.Register("21", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewIterationNodeParser(vrs)
	})

	// Register Selector node parser (Phase 4)
	factory.Register("8", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewSelectorNodeParser(vrs)
	})

	// Register string type names for compatibility with newer Coze exports
	factory.Register("start", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewStartNodeParser(vrs)
	})
	factory.Register("end", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewEndNodeParser(vrs)
	})
	factory.Register("llm", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewLLMNodeParser(vrs)
	})
	factory.Register("code", func(vrs *models.VariableReferenceSystem) NodeParser {
		return NewCodeNodeParser(vrs)
	})

	return factory
}

// Register registers a parser.
func (f *ParserFactory) Register(nodeType string, creator func(*models.VariableReferenceSystem) NodeParser) {
	f.parsers[nodeType] = creator
}

// CreateParser creates a parser.
func (f *ParserFactory) CreateParser(nodeType string, variableRefSystem *models.VariableReferenceSystem) (NodeParser, error) {
	creator, exists := f.parsers[nodeType]
	if !exists {
		return nil, fmt.Errorf("no parser found for node type: %s", nodeType)
	}

	return creator(variableRefSystem), nil
}

// CreateParserWithFallback creates a parser with graceful fallback support
func (f *ParserFactory) CreateParserWithFallback(nodeType string, variableRefSystem *models.VariableReferenceSystem) (NodeParser, bool, error) {
	creator, exists := f.parsers[nodeType]
	if !exists {
		return nil, false, nil // Not supported but don't error
	}
	parser := creator(variableRefSystem)
	return parser, true, nil
}

// ParseNodeWithFallback parses a node using fallback mechanism
func (f *ParserFactory) ParseNodeWithFallback(cozeNode CozeNode, variableRefSystem *models.VariableReferenceSystem) (*models.Node, bool, error) {
	parser, supported, err := f.CreateParserWithFallback(cozeNode.Type, variableRefSystem)
	if err != nil {
		return nil, false, err
	}
	if !supported {
		return nil, false, nil // Node type not supported, return flag
	}

	node, err := parser.ParseNode(cozeNode)
	return node, true, err
}

// GetSupportedTypes returns supported node types.
func (f *ParserFactory) GetSupportedTypes() []string {
	types := make([]string, 0, len(f.parsers))
	for nodeType := range f.parsers {
		types = append(types, nodeType)
	}
	return types
}
