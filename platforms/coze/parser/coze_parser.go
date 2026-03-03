package parser

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/iflytek/agentbridge/core/interfaces"
	"github.com/iflytek/agentbridge/internal/models"
	"github.com/iflytek/agentbridge/platforms/common"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Compile-time interface check
var _ interfaces.DSLParser = (*CozeParser)(nil)

// CozeParser parses Coze DSL to unified format.
type CozeParser struct {
	*common.BaseParser
	factory           *ParserFactory
	variableRefSystem *models.VariableReferenceSystem
	skippedNodeIDs    map[string]bool // Track skipped node IDs
	cozeDSL           *CozeDSL        // Reference to complete DSL for enhancement
	verbose           bool            // Verbose mode flag
}

func NewCozeParser() *CozeParser {
	variableRefSystem := models.NewVariableReferenceSystem()

	return &CozeParser{
		BaseParser:        common.NewBaseParser(models.PlatformCoze),
		factory:           NewParserFactory(),
		variableRefSystem: variableRefSystem,
		verbose:           false, // Default to non-verbose
	}
}

// SetVerbose sets the verbose mode for debugging output
func (p *CozeParser) SetVerbose(verbose bool) {
	p.verbose = verbose
}

// debugPrintf prints debug messages only in verbose mode
func (p *CozeParser) debugPrintf(format string, args ...interface{}) {
	if p.verbose {
		fmt.Printf("🔧 "+format, args...)
	}
}

// Parse parses Coze DSL to unified format.
func (p *CozeParser) Parse(data []byte) (*models.UnifiedDSL, error) {
	// Detect format and convert ZIP to YAML if needed
	if p.isZipFormat(data) {
		p.debugPrintf("Detected ZIP format, converting to YAML\n")

		yamlData, err := p.parseZipToYaml(data)
		if err != nil {
			return nil, fmt.Errorf("failed to convert ZIP to YAML: %w", err)
		}
		// Recursively parse YAML using standard parsing logic
		return p.Parse(yamlData)
	}

	// Parse YAML format using standard logic
	// Validate input data
	if err := p.Validate(data); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Parse YAML
	var cozeDSL CozeDSL
	if err := yaml.Unmarshal(data, &cozeDSL); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	// Store reference to cozeDSL for data enhancement
	p.cozeDSL = &cozeDSL

	// Build unified DSL
	unifiedDSL := &models.UnifiedDSL{
		Version: "1.0",
		Metadata: models.Metadata{
			Name:        cozeDSL.Name,
			Description: cozeDSL.Description,
			CreatedAt:   time.Unix(cozeDSL.CreateTime, 0),
			UpdatedAt:   time.Unix(cozeDSL.UpdateTime, 0),
		},
		PlatformMetadata: models.PlatformMetadata{
			// TODO: Add Coze-specific metadata structure when needed
		},
		Workflow: models.Workflow{
			Nodes:     []models.Node{},
			Edges:     []models.Edge{},
			Variables: []models.Variable{},
		},
	}

	// Parse nodes using root level nodes with complete configuration as primary source
	var allNodes []CozeNode

	// Use root level nodes containing complete detailed information
	if len(cozeDSL.Nodes) > 0 {
		allNodes = append(allNodes, cozeDSL.Nodes...)
	} else if len(cozeDSL.Schema.Nodes) > 0 {
		// Fallback to schema nodes when root level nodes are unavailable
		// Convert CozeSchemaNode to CozeNode
		for _, schemaNode := range cozeDSL.Schema.Nodes {
			// Convert CozeSchemaNodeData to CozeNodeData structure
			nodeData := CozeNodeData{
				// Map nodeMeta to meta fields
				Meta: CozeDataMeta{
					Title:       schemaNode.Data.NodeMeta.Title,
					Description: schemaNode.Data.NodeMeta.Description,
					Icon:        schemaNode.Data.NodeMeta.Icon,
					Subtitle:    schemaNode.Data.NodeMeta.SubTitle,
				},
				Outputs: schemaNode.Data.Outputs,
			}

			// Convert inputs if present
			if schemaNode.Data.Inputs != nil {
				nodeInputs := &CozeNodeInputs{}

				// Map the inputs structure
				if inputParams := schemaNode.Data.Inputs; inputParams != nil {
					// Convert CozeInputParam to CozeNodeInputParam structure
					for _, inputParam := range inputParams.InputParameters {
						nodeInputParam := CozeNodeInputParam{
							Name: inputParam.Name,
							Input: CozeNodeInput{
								Type: inputParam.Input.Type,
								// Map other fields as needed
							},
						}
						nodeInputs.InputParameters = append(nodeInputs.InputParameters, nodeInputParam)
					}
				}

				nodeData.Inputs = nodeInputs
			}

			cozeNode := CozeNode{
				ID:   schemaNode.ID,
				Type: schemaNode.Type,
				Data: nodeData,
				// Extract edges if they exist in the node
				Edges: schemaNode.Edges,
			}
			allNodes = append(allNodes, cozeNode)
		}
	}

	if err := p.parseNodes(allNodes, unifiedDSL); err != nil {
		return nil, fmt.Errorf("failed to parse nodes: %w", err)
	}

	// Parse main layer connection relationships using root level edges
	var mainLayerEdges []CozeEdge

	// Check for edges in the root level containing complete data
	if len(cozeDSL.Edges) > 0 {
		// Convert CozeRootEdge to CozeEdge format
		for _, rootEdge := range cozeDSL.Edges {
			cozeEdge := CozeEdge{
				FromNode: rootEdge.FromNode,
				FromPort: rootEdge.FromPort,
				ToNode:   rootEdge.ToNode,
				ToPort:   rootEdge.ToPort,
			}
			mainLayerEdges = append(mainLayerEdges, cozeEdge)
		}
	} else if len(cozeDSL.Schema.Edges) > 0 {
		// Fallback to schema edges when root level edges are unavailable
		// Convert CozeSchemaEdge to CozeEdge - ONLY main layer edges
		for _, schemaEdge := range cozeDSL.Schema.Edges {
			cozeEdge := CozeEdge{
				FromNode: schemaEdge.SourceNodeID,
				FromPort: schemaEdge.SourcePortID,
				ToNode:   schemaEdge.TargetNodeID,
				ToPort:   schemaEdge.TargetPortID,
			}
			mainLayerEdges = append(mainLayerEdges, cozeEdge)
		}
	}

	if err := p.parseMainLayerEdges(mainLayerEdges, unifiedDSL); err != nil {
		return nil, fmt.Errorf("failed to parse main layer edges: %w", err)
	}

	// Parse iteration internal edges
	if err := p.parseIterationInternalEdges(allNodes, unifiedDSL); err != nil {
		return nil, fmt.Errorf("failed to parse iteration internal edges: %w", err)
	}

	// Print conversion summary after parsing is complete
	p.printConversionSummary(unifiedDSL)

	return unifiedDSL, nil
}

// Helper function to safely get string values from maps
func (p *CozeParser) getStringFromMap(m map[string]interface{}, key string, defaultValue string) string {
	if value, exists := m[key]; exists {
		if strVal, ok := value.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// Validate validates Coze DSL format.
func (p *CozeParser) Validate(data []byte) error {
	// Support validating Coze ZIP by converting to YAML first
	if p.isZipFormat(data) {
		p.debugPrintf("Detected ZIP format in Validate, converting to YAML\n")
		yamlData, err := p.parseZipToYaml(data)
		if err != nil {
			return fmt.Errorf("invalid ZIP format: %w", err)
		}
		data = yamlData
	}

	var cozeDSL CozeDSL
	if err := yaml.Unmarshal(data, &cozeDSL); err != nil {
		return fmt.Errorf("invalid YAML format: %w", err)
	}

	// Validate required fields
	if cozeDSL.Name == "" {
		return fmt.Errorf("workflow name is required")
	}

	if cozeDSL.Nodes == nil {
		return fmt.Errorf("workflow nodes are required")
	}

	return nil
}

// parseNodes parses nodes.
func (p *CozeParser) parseNodes(cozeNodes []CozeNode, unifiedDSL *models.UnifiedDSL) error {
	// Track skipped node IDs for edge filtering
	skippedNodeIDs := make(map[string]bool)
	p.skippedNodeIDs = skippedNodeIDs // Ensure the parser instance has access to skipped node IDs

	// Pre-register output mappings from iteration nodes for reference resolution
	p.preRegisterIterationOutputMappings(cozeNodes)

	for _, cozeNode := range cozeNodes {
		// Normalize node structure to handle both old and new formats
		p.normalizeNodeStructure(&cozeNode)

		// Enhance iteration nodes with complete data from schema
		if cozeNode.Type == "21" {
			p.enhanceIterationNodeWithCompleteData(&cozeNode, p.cozeDSL)
		}

		// Use fallback parsing to handle unsupported node types
		node, supported, err := p.factory.ParseNodeWithFallback(cozeNode, p.variableRefSystem)
		if err != nil {
			return fmt.Errorf("failed to parse node %s: %w", cozeNode.ID, err)
		}

		if !supported {
			// Convert unsupported nodes to code node placeholders
			fmt.Printf("⚠️  Converting unsupported node type '%s' (ID: %s) to code node placeholder\n",
				cozeNode.Type, cozeNode.ID)

			node, err = p.convertUnsupportedNodeToCodeNode(cozeNode)
			if err != nil {
				return fmt.Errorf("failed to convert unsupported node %s: %w", cozeNode.ID, err)
			}
		}

		// If it's an end node, we need to pass skipped node IDs to handle references correctly
		if node.Type == models.NodeTypeEnd {
			// Re-parse with end node parser context
			endParser := NewEndNodeParser(p.variableRefSystem).(*EndNodeParser)
			endParser.SetSkippedNodeIDs(skippedNodeIDs)
			node, err = endParser.ParseNode(cozeNode)
			if err != nil {
				return fmt.Errorf("failed to parse end node %s: %w", cozeNode.ID, err)
			}
		}

		unifiedDSL.Workflow.Nodes = append(unifiedDSL.Workflow.Nodes, *node)

		// If this is an iteration node, also add its sub-nodes to the main node list
		if node.Type == models.NodeTypeIteration {
			if iterationConfig, ok := node.Config.(models.IterationConfig); ok {
				for _, subNode := range iterationConfig.SubWorkflow.Nodes {
					// Create copy of sub-node to avoid modifying original
					subNodeCopy := subNode
					unifiedDSL.Workflow.Nodes = append(unifiedDSL.Workflow.Nodes, subNodeCopy)
				}
			}
		}
	}

	// Save skipped node IDs for edge parsing
	p.skippedNodeIDs = skippedNodeIDs

	return nil
}

// enhanceIterationNodeWithCompleteData enhances iteration node blocks with complete schema data
func (p *CozeParser) enhanceIterationNodeWithCompleteData(iterationNode *CozeNode, cozeDSL *CozeDSL) {

	// Find the complete iteration node data in schema.nodes
	var completeIterationNode *CozeSchemaNode
	for _, schemaNode := range cozeDSL.Schema.Nodes {
		if schemaNode.ID == iterationNode.ID {
			completeIterationNode = &schemaNode
			break
		}
	}

	if completeIterationNode == nil {
		return
	}

	// Build a map of internal node ID to complete data from schema iteration node
	internalNodeMap := make(map[string]interface{})
	for _, block := range completeIterationNode.Blocks {
		if blockMap, ok := block.(map[string]interface{}); ok {
			blockID := p.getStringFromMap(blockMap, "id", "")
			if blockID != "" {
				internalNodeMap[blockID] = block
			}
		}
	}

	// Enhance each block in the iteration node with complete schema data
	for i, blockInterface := range iterationNode.Blocks {
		if blockMap, ok := blockInterface.(map[string]interface{}); ok {
			blockID := p.getStringFromMap(blockMap, "id", "")
			blockType := p.getStringFromMap(blockMap, "type", "")

			if blockID == "" {
				continue
			}

			// Find complete internal node data from schema
			if completeBlockData, found := internalNodeMap[blockID]; found {
				if completeBlockMap, ok := completeBlockData.(map[string]interface{}); ok {
					// Enhance the block with complete schema data
					if blockType == "3" {
						p.enhanceLLMBlockWithSchemaData(blockMap, completeBlockMap)
					}
				}

				// Update the block in the iteration node
				iterationNode.Blocks[i] = blockMap
			} else {
			}
		}
	}
}

// enhanceLLMBlockWithSchemaData enhances LLM block data with complete schema data
func (p *CozeParser) enhanceLLMBlockWithSchemaData(blockMap map[string]interface{}, schemaBlockMap map[string]interface{}) {

	// Extract complete LLM parameters from schema block
	if schemaData, hasData := schemaBlockMap["data"].(map[string]interface{}); hasData {
		if schemaInputs, hasInputs := schemaData["inputs"].(map[string]interface{}); hasInputs {
			if llmParam, hasLLMParam := schemaInputs["llmParam"]; hasLLMParam {

				// Ensure the target block has data.inputs structure
				if blockData, hasBlockData := blockMap["data"].(map[string]interface{}); hasBlockData {
					if blockInputs, hasBlockInputs := blockData["inputs"].(map[string]interface{}); hasBlockInputs {
						// Replace incomplete LLM params with complete schema ones
						blockInputs["llmparam"] = llmParam
					} else {
						// Create inputs structure if it doesn't exist
						blockData["inputs"] = map[string]interface{}{
							"llmparam": llmParam,
						}
					}
				} else {
					// Create data structure if it doesn't exist
					blockMap["data"] = map[string]interface{}{
						"inputs": map[string]interface{}{
							"llmparam": llmParam,
						},
					}
				}
			}
		}
	}
}

// preRegisterIterationOutputMappings pre-registers output mappings from iteration nodes
// This ensures mappings are available before other nodes that reference iteration outputs are parsed
func (p *CozeParser) preRegisterIterationOutputMappings(cozeNodes []CozeNode) {
	for _, cozeNode := range cozeNodes {
		// Only process iteration nodes (type "21")
		if cozeNode.Type == "21" {
			// Pre-register the standard iteration output mapping: result_list -> output
			if cozeNode.Data.Outputs != nil {
				for _, originalOutput := range cozeNode.Data.Outputs {
					// Register mapping from original Coze output name to standardized iFlytek name
					if originalOutput.Name != "output" && p.variableRefSystem != nil {
						p.variableRefSystem.RegisterOutputMapping(cozeNode.ID, originalOutput.Name, "output")
					}
				}
			}
		}
	}
}

// parseMainLayerEdges parses main layer connection relationships.
func (p *CozeParser) parseMainLayerEdges(cozeEdges []CozeEdge, unifiedDSL *models.UnifiedDSL) error {

	for _, cozeEdge := range cozeEdges {

		// Skip edges with empty node IDs - these are invalid
		if cozeEdge.FromNode == "" || cozeEdge.ToNode == "" {
			continue
		}

		// Skip edges involving filtered nodes
		if p.skippedNodeIDs != nil {
			if p.skippedNodeIDs[cozeEdge.FromNode] || p.skippedNodeIDs[cozeEdge.ToNode] {
				continue
			}
		}

		var edge models.Edge
		edge = models.Edge{
			ID:           fmt.Sprintf("edge-%s-%s", cozeEdge.FromNode, cozeEdge.ToNode),
			Source:       cozeEdge.FromNode,
			Target:       cozeEdge.ToNode,
			SourceHandle: p.convertCozeSourceHandle(cozeEdge.FromPort, cozeEdge.FromNode, unifiedDSL),
			TargetHandle: cozeEdge.ToPort,
			Type:         models.EdgeTypeDefault, // Coze uses default edge type
		}

		// Parse platform-specific configuration
		edge.PlatformConfig = models.PlatformConfig{
			IFlytek: make(map[string]interface{}),
			Dify:    make(map[string]interface{}),
		}

		unifiedDSL.Workflow.Edges = append(unifiedDSL.Workflow.Edges, edge)
	}

	return nil
}

// convertCozeSourceHandle converts Coze branch format to unified DSL format dynamically
func (p *CozeParser) convertCozeSourceHandle(fromPort string, sourceNodeID string, unifiedDSL *models.UnifiedDSL) string {
	// Handle default case
	if fromPort == "default" {
		return "default"
	}

	// Find the source node in unified DSL
	var sourceNode *models.Node
	for i := range unifiedDSL.Workflow.Nodes {
		if unifiedDSL.Workflow.Nodes[i].ID == sourceNodeID {
			sourceNode = &unifiedDSL.Workflow.Nodes[i]
			break
		}
	}

	if sourceNode == nil {
		return fromPort
	}

	// Handle selector node (NodeTypeCondition) branch formats
	if sourceNode.Type == models.NodeTypeCondition {
		return p.convertSelectorBranchHandle(fromPort, sourceNode)
	}

	// Handle classifier node branch formats
	if sourceNode.Type == models.NodeTypeClassifier && strings.HasPrefix(fromPort, "branch_") {
		return p.convertClassifierBranchHandle(fromPort, sourceNode)
	}

	// Return as-is if no conversion needed
	return fromPort
}

// convertSelectorBranchHandle converts selector node branch handles
func (p *CozeParser) convertSelectorBranchHandle(fromPort string, sourceNode *models.Node) string {
	// Handle "true" (first branch) -> case_0
	if fromPort == "true" {
		return "case_0"
	}

	// Handle "false" (default branch)
	if fromPort == "false" {
		return "__default__"
	}

	// Handle "true_X" format using same logic as main layer selector parser
	// true_1 -> case_1, true_2 -> case_2, etc.
	if strings.HasPrefix(fromPort, "true_") {
		indexStr := strings.TrimPrefix(fromPort, "true_")
		if index, err := strconv.Atoi(indexStr); err == nil {
			// true_1 maps to case_1, true_2 maps to case_2, etc.
			// This matches the main layer selector parser logic: fmt.Sprintf("case_%d", index)
			// BUT: true_1 should map to the SECOND case (case_1), not case_2
			// The index in true_X is 1-based but we need it to map correctly to case IDs
			return fmt.Sprintf("case_%d", index)
		}
	}

	return fromPort
}

// convertClassifierBranchHandle converts classifier node branch handles
func (p *CozeParser) convertClassifierBranchHandle(fromPort string, sourceNode *models.Node) string {
	classifierConfig, ok := sourceNode.Config.(models.ClassifierConfig)
	if !ok {
		return p.convertBranchToNumeric(fromPort)
	}

	// Extract branch index
	branchIndexStr := strings.TrimPrefix(fromPort, "branch_")
	branchIndex, err := strconv.Atoi(branchIndexStr)
	if err != nil {
		return fromPort
	}

	// Map to 1-based index for iFlytek format
	if branchIndex >= 0 && branchIndex < len(classifierConfig.Classes) {
		return fmt.Sprintf("%d", branchIndex+1)
	}

	return p.convertBranchToNumeric(fromPort)
}

// parseIterationInternalEdges parses edges inside iteration nodes
func (p *CozeParser) parseIterationInternalEdges(cozeNodes []CozeNode, unifiedDSL *models.UnifiedDSL) error {
	for _, cozeNode := range cozeNodes {
		// Only process iteration nodes (type "21")
		if cozeNode.Type == "21" && len(cozeNode.Edges) > 0 {

			for _, edgeInterface := range cozeNode.Edges {
				if edgeMap, ok := edgeInterface.(map[string]interface{}); ok {
					// Create CozeEdge from the map
					fromNode := p.getStringFromEdgeMap(edgeMap, "sourcenodeid")
					toNode := p.getStringFromEdgeMap(edgeMap, "targetnodeid")
					fromPort := p.getStringFromEdgeMap(edgeMap, "sourceportid")
					toPort := p.getStringFromEdgeMap(edgeMap, "targetportid")

					// Skip edges with empty node IDs - these are invalid
					if fromNode == "" || toNode == "" {
						continue
					}

					// Skip edges involving non-existent nodes (filtered out as unsupported)
					if !p.nodeExistsInDSL(fromNode, unifiedDSL) || !p.nodeExistsInDSL(toNode, unifiedDSL) {
						continue
					}

					cozeEdge := CozeEdge{
						FromNode: fromNode,
						ToNode:   toNode,
						FromPort: fromPort,
						ToPort:   toPort,
					}

					// Skip loop-function-inline-input edges (back to iteration node)
					if cozeEdge.ToPort == "loop-function-inline-input" {
						continue
					}

					var edge models.Edge
					// Convert loop-function-inline-output edges to iteration start edges
					if cozeEdge.FromPort == "loop-function-inline-output" {
						// This edge represents connection from iteration start node to internal processing node
						// We keep the iteration node ID as source, but mark it for special processing
						// The iFlytek generator will map it to the correct iteration start node ID

						edge = models.Edge{
							ID:           fmt.Sprintf("edge-iteration-start-%s-%s", cozeEdge.FromNode, cozeEdge.ToNode),
							Source:       cozeEdge.FromNode, // Keep iteration node ID - generator will map it
							Target:       cozeEdge.ToNode,   // Target should be the processing node (e.g., code node)
							SourceHandle: "",                // No special handle - generator will handle
							TargetHandle: cozeEdge.ToPort,
							Type:         models.EdgeTypeDefault,
						}

						// Add metadata to help iFlytek generator recognize this as iteration start edge
						edge.PlatformConfig = models.PlatformConfig{
							IFlytek: map[string]interface{}{
								"isIterationStartEdge": true,
								"originalFromPort":     cozeEdge.FromPort,
								"iterationNodeID":      cozeEdge.FromNode,
							},
							Dify: make(map[string]interface{}),
						}
					} else {
						// Regular internal edge
						edge = models.Edge{
							ID:           fmt.Sprintf("edge-%s-%s", cozeEdge.FromNode, cozeEdge.ToNode),
							Source:       cozeEdge.FromNode,
							Target:       cozeEdge.ToNode,
							SourceHandle: p.convertCozeSourceHandle(cozeEdge.FromPort, cozeEdge.FromNode, unifiedDSL),
							TargetHandle: cozeEdge.ToPort,
							Type:         models.EdgeTypeDefault,
						}

						edge.PlatformConfig = models.PlatformConfig{
							IFlytek: make(map[string]interface{}),
							Dify:    make(map[string]interface{}),
						}
					}

					unifiedDSL.Workflow.Edges = append(unifiedDSL.Workflow.Edges, edge)
				}
			}
		}
	}
	return nil
}

// getStringFromEdgeMap safely extracts string value from edge map
func (p *CozeParser) getStringFromEdgeMap(edgeMap map[string]interface{}, key string) string {
	if value, exists := edgeMap[key]; exists {
		if strValue, ok := value.(string); ok {
			return strValue
		}
	}
	return ""
}

// convertBranchToNumeric provides fallback numeric conversion for branch format
func (p *CozeParser) convertBranchToNumeric(fromPort string) string {
	switch fromPort {
	case "branch_0":
		return "1"
	case "branch_1":
		return "2"
	case "branch_2":
		return "3"
	case "branch_3":
		return "4"
	default:
		return fromPort
	}
}

// convertUnsupportedNodeToCodeNode converts unsupported nodes to code node placeholders
func (p *CozeParser) convertUnsupportedNodeToCodeNode(cozeNode CozeNode) (*models.Node, error) {
	// Get node title for type description
	nodeTitle := p.extractNodeTitle(cozeNode)

	// Use code node parser to create code node
	codeParser := NewCodeNodeParser(p.variableRefSystem)

	// Create modified node with code node type
	modifiedNode := cozeNode
	modifiedNode.Type = "5" // Coze code node type

	modifiedNode.Data.Meta.Title = fmt.Sprintf("暂不兼容的节点-%s（请根据需求手动实现）", nodeTitle)

	// Set default code configuration
	if modifiedNode.Data.Inputs == nil {
		modifiedNode.Data.Inputs = &CozeNodeInputs{}
	}

	// Create code runner configuration
	codeRunnerConfig := make(map[string]interface{})
	codeRunnerConfig["code"] = fmt.Sprintf(`# 抱歉！当前兼容性工具不支持转换此类节点: %s

# 请根据业务需求手动补充实现逻辑`, nodeTitle)
	codeRunnerConfig["language"] = "python3"
	modifiedNode.Data.Inputs.CodeRunner = codeRunnerConfig

	// Clear input parameters to avoid CodeNodeParser generating parameterized function signature
	modifiedNode.Data.Inputs.InputParameters = []CozeNodeInputParam{}

	// Create default output if none exist to maintain connections
	if len(modifiedNode.Data.Outputs) == 0 {
		defaultOutput := CozeOutput{
			Name:     "result",
			Type:     "string",
			Required: false,
			Schema:   map[string]interface{}{"type": "string", "default": ""},
		}
		modifiedNode.Data.Outputs = []CozeOutput{defaultOutput}
	}

	// Parse using code node parser
	return codeParser.ParseNode(modifiedNode)
}

// extractNodeTitle extracts node title
func (p *CozeParser) extractNodeTitle(cozeNode CozeNode) string {
	if cozeNode.Data.Meta.Title != "" {
		return cozeNode.Data.Meta.Title
	}
	return cozeNode.Type
}

// nodeExistsInDSL checks if a node exists in the unified DSL
func (p *CozeParser) nodeExistsInDSL(nodeID string, unifiedDSL *models.UnifiedDSL) bool {
	for _, node := range unifiedDSL.Workflow.Nodes {
		if node.ID == nodeID {
			return true
		}
	}
	return false
}
func (p *CozeParser) printConversionSummary(unifiedDSL *models.UnifiedDSL) {
	totalNodes := len(unifiedDSL.Workflow.Nodes)
	fmt.Printf("✅ Conversion Summary: All %d nodes processed successfully\n", totalNodes)

	// Count nodes converted to code placeholders by checking titles
	convertedCount := 0
	for _, node := range unifiedDSL.Workflow.Nodes {
		if strings.Contains(node.Title, "暂不兼容的节点-") {
			convertedCount++
		}
	}

	if convertedCount > 0 {
		fmt.Printf("ℹ️  %d unsupported nodes were converted to code node placeholders\n", convertedCount)
		fmt.Printf("ℹ️  Please manually adjust these placeholder nodes as needed\n")
	}
}

// isZipFormat detects whether data is in ZIP format
func (p *CozeParser) isZipFormat(data []byte) bool {
	// Check ZIP file header (PK)
	if len(data) < 4 {
		return false
	}

	// ZIP file magic number: 0x504B (PK)
	if data[0] == 0x50 && data[1] == 0x4B {
		return true
	}

	// Check Base64-encoded ZIP (usually starts with UEs)
	if len(data) > 10 {
		dataStr := string(data[:50]) // Check first 50 characters
		if strings.HasPrefix(dataStr, "UEs") || p.isBase64Encoded(data) {
			return true
		}
	}

	return false
}

// parseZipToYaml converts ZIP format to YAML format following Coze source implementation
func (p *CozeParser) parseZipToYaml(data []byte) ([]byte, error) {
	p.debugPrintf("Starting ZIP to YAML conversion\n")

	// Step 1: Handle Base64 decoding following Coze source logic
	zipBytes, err := p.decodeZipData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ZIP data: %w", err)
	}

	// Step 2: Extract workflow JSON data following Coze source logic
	jsonData, manifestData, err := p.extractWorkflowDataFromZip(zipBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to extract workflow data: %w", err)
	}

	// Step 3: Convert to CozeDSL structure
	cozeDSL, err := p.convertToCozeDSL(jsonData, manifestData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to CozeDSL: %w", err)
	}

	// Step 4: Serialize to YAML
	yamlBytes, err := yaml.Marshal(cozeDSL)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML: %w", err)
	}

	p.debugPrintf("ZIP to YAML conversion completed, YAML size: %d bytes\n", len(yamlBytes))

	// Debug: Save intermediate results to file
	if err := os.MkdirAll("../../test_output", 0755); err == nil {
		if err := os.WriteFile("../../test_output/debug_coze_dsl.yml", yamlBytes, 0644); err != nil {
		} else {
		}
	}

	return yamlBytes, nil
}

// decodeZipData handles Base64-encoded ZIP data following Coze source logic
func (p *CozeParser) decodeZipData(data []byte) ([]byte, error) {
	// Check if data is Base64 encoded
	if !p.isBase64Encoded(data) {
		// Return raw ZIP data directly
		p.debugPrintf("Data is raw ZIP format\n")
		return data, nil
	}

	// Base64 decode following Coze source workflow implementation
	p.debugPrintf("Decoding Base64 ZIP data\n")

	// Optimization: Estimate decoded size to reduce allocations
	encodedLen := len(data)
	decodedLen := base64.StdEncoding.DecodedLen(encodedLen)

	// Pre-allocate buffer for better performance
	decoded := make([]byte, decodedLen)
	n, err := base64.StdEncoding.Decode(decoded, data)
	if err != nil {
		// Fallback to standard decode for compatibility
		decoded, err = base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64: %w", err)
		}
	} else {
		// Trim to actual decoded length
		decoded = decoded[:n]
	}

	p.debugPrintf("Base64 decoded successfully, size: %d bytes\n", len(decoded))
	return decoded, nil
}

// isBase64Encoded checks if data is Base64 encoded
func (p *CozeParser) isBase64Encoded(data []byte) bool {
	// Check Base64 character set
	str := string(data)
	if len(str) < 10 {
		return false
	}

	// Check first 100 characters for Base64 format
	checkStr := str
	if len(checkStr) > 100 {
		checkStr = str[:100]
	}

	// Base64-encoded ZIP usually starts with UEs, or conforms to Base64 character set
	if strings.HasPrefix(checkStr, "UEs") {
		return true
	}

	// Check Base64 character set compliance
	matched, _ := regexp.MatchString(`^[A-Za-z0-9+/]*={0,2}$`, strings.TrimSpace(checkStr))
	return matched
}

// extractWorkflowDataFromZip extracts workflow data from ZIP file following Coze source logic
func (p *CozeParser) extractWorkflowDataFromZip(zipBytes []byte) (map[string]interface{}, map[string]interface{}, error) {
	// Create ZIP reader following Coze source workflow implementation
	// Optimization: Use readerAt for better performance
	reader := bytes.NewReader(zipBytes)
	zipReader, err := zip.NewReader(reader, int64(len(zipBytes)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read ZIP file: %w", err)
	}

	// Pre-allocate buffer to reduce memory allocations
	var workflowContent strings.Builder

	// Traverse ZIP file contents following Coze source logic
	for _, file := range zipReader.File {
		p.debugPrintf("Processing ZIP entry: %s, size: %d\n", file.Name, file.UncompressedSize64)

		// Find workflow files first to avoid unnecessary reads
		if !strings.Contains(file.Name, "Workflow-") || !strings.HasSuffix(file.Name, ".zip") {
			continue
		}

		reader, err := file.Open()
		if err != nil {
			p.debugPrintf("Failed to open ZIP entry %s: %v\n", file.Name, err)
			continue
		}

		// Optimization: Pre-allocate buffer based on file size
		workflowContent.Grow(int(file.UncompressedSize64))

		// Use efficient copying with pre-allocated buffer
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			p.debugPrintf("Failed to read ZIP entry %s: %v\n", file.Name, err)
			continue
		}

		// Write content to builder for efficiency
		workflowContent.Write(content)

		p.debugPrintf("Found workflow content in: %s\n", file.Name)
		break
	}

	if workflowContent.Len() == 0 {
		return nil, nil, fmt.Errorf("no workflow content found in ZIP")
	}

	// Extract JSON and MANIFEST data following Coze source logic
	return p.extractWorkflowDataFromContent(workflowContent.String())
}

// extractWorkflowDataFromContent extracts workflow data and MANIFEST from content following Coze source workflow
func (p *CozeParser) extractWorkflowDataFromContent(content string) (map[string]interface{}, map[string]interface{}, error) {
	// JSON boundary detection algorithm following Coze source workflow implementation

	// Step 1: Find JSON start position
	jsonStart := strings.Index(content, `{"edges"`)
	if jsonStart == -1 {
		jsonStart = strings.Index(content, `{"nodes"`)
	}
	if jsonStart == -1 {
		return nil, nil, fmt.Errorf("no JSON start found")
	}

	// Step 2: Extract content from JSON start position
	contentFromJson := content[jsonStart:]

	// Step 3: Simple bracket matching algorithm following Coze source workflow logic
	braceCount := 0
	jsonEnd := -1
	for i, char := range contentFromJson {
		if char == '{' {
			braceCount++
		} else if char == '}' {
			braceCount--
			if braceCount == 0 {
				jsonEnd = i
				break
			}
		}
	}

	if jsonEnd == -1 {
		return nil, nil, fmt.Errorf("no complete JSON found, unmatched braces")
	}

	// Step 4: JSON data cleaning following Coze source logic
	jsonMatch := contentFromJson[:jsonEnd+1]
	cleanJsonString := p.cleanJsonString(jsonMatch)

	// Step 5: Parse JSON data
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(cleanJsonString), &jsonData); err != nil {
		return nil, nil, fmt.Errorf("failed to parse JSON data: %w", err)
	}

	// Step 6: Parse MANIFEST.yml
	manifestData, err := p.parseManifestFromContent(content)
	if err != nil {
		// MANIFEST parsing failure is not fatal, use default values
		manifestData = make(map[string]interface{})
	}

	return jsonData, manifestData, nil
}

// cleanJsonString cleans JSON string following Coze source workflow logic
func (p *CozeParser) cleanJsonString(jsonStr string) string {
	// Step 1: Remove null characters
	cleaned := strings.ReplaceAll(jsonStr, "\x00", "")

	// Step 2: Remove all control characters
	cleaned = regexp.MustCompile(`[\x00-\x1F\x7F-\x9F]`).ReplaceAllString(cleaned, "")

	// Step 3: Trim leading and trailing whitespace
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

// parseManifestFromContent parses MANIFEST.yml from content following Coze source workflow logic
func (p *CozeParser) parseManifestFromContent(content string) (map[string]interface{}, error) {
	// Use regex and parsing logic identical to Coze source
	manifestRegex := regexp.MustCompile(`MANIFEST\.yml[\s\S]*?type:\s*(\w+)[\s\S]*?version:\s*([^\n\r]+)[\s\S]*?main:\s*[\s\S]*?id:\s*([^\n\r]+)[\s\S]*?name:\s*([^\n\r]+)[\s\S]*?desc:\s*([^\n\r]+)`)
	manifestMatch := manifestRegex.FindStringSubmatch(content)

	if len(manifestMatch) < 6 {
		return nil, fmt.Errorf("MANIFEST.yml format not recognized")
	}

	manifestData := map[string]interface{}{
		"type":    strings.TrimSpace(manifestMatch[1]),
		"version": strings.Trim(strings.TrimSpace(manifestMatch[2]), `"`),
		"main": map[string]interface{}{
			"id":   strings.Trim(strings.TrimSpace(manifestMatch[3]), `"`),
			"name": strings.Trim(strings.TrimSpace(manifestMatch[4]), `"`),
			"desc": strings.Trim(strings.TrimSpace(manifestMatch[5]), `"`),
		},
	}

	return manifestData, nil
}

// getMapKeys gets all keys from map (helper debug function)
func (p *CozeParser) getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// convertToCozeDSL converts JSON data to CozeDSL structure referencing Coze source convertZipWorkflowToOpenSource logic
func (p *CozeParser) convertToCozeDSL(jsonData map[string]interface{}, manifestData map[string]interface{}) (*CozeDSL, error) {
	currentTime := time.Now().Unix()

	// Build basic CozeDSL structure
	cozeDSL := &CozeDSL{
		WorkflowID:     fmt.Sprintf("imported_%d", time.Now().Unix()),
		Name:           p.extractNameFromManifest(manifestData),
		Description:    p.extractDescFromManifest(manifestData),
		Version:        p.extractVersionFromManifest(manifestData),
		CreateTime:     currentTime,
		UpdateTime:     currentTime,
		ExportFormat:   "yaml",
		SerializedData: "", // Avoid circular references
	}

	// Convert node data following Coze source logic
	nodes, err := p.convertNodes(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert nodes: %w", err)
	}
	cozeDSL.Nodes = nodes

	// Convert edge data following Coze source logic
	edges, err := p.convertEdges(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert edges: %w", err)
	}
	cozeDSL.Edges = edges

	// Build Schema for internal compatibility
	cozeDSL.Schema = p.buildSchema(jsonData)

	// Build metadata and dependencies
	cozeDSL.Metadata = p.buildMetadata()
	cozeDSL.Dependencies = p.buildDependencies(nodes)

	return cozeDSL, nil
}

// convertNodes converts node data following Coze source workflow logic
func (p *CozeParser) convertNodes(jsonData map[string]interface{}) ([]CozeNode, error) {
	var convertedNodes []CozeNode

	nodesData, ok := jsonData["nodes"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid nodes data format")
	}

	for _, node := range nodesData {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}

		// Following Coze source workflow logic: remove blocks field
		nodeWithoutBlocks := make(map[string]interface{})
		for k, v := range nodeMap {
			if k != "blocks" {
				nodeWithoutBlocks[k] = v
			}
		}

		// Create CozeNode structure
		convertedNode := CozeNode{}

		// Basic field mapping
		if id, ok := nodeWithoutBlocks["id"].(string); ok {
			convertedNode.ID = id
		}
		if nodeType, ok := nodeWithoutBlocks["type"].(string); ok {
			convertedNode.Type = nodeType
		}

		// Convert node data from filtered nodeMap
		convertedNode.Data = p.extractNodeDataFromMap(nodeWithoutBlocks)

		// Convert metadata
		convertedNode.Meta = p.extractNodeMetaFromMap(nodeWithoutBlocks)

		// Handle edges field for iteration nodes - note: do not remove edges field
		if edges, ok := nodeWithoutBlocks["edges"].([]interface{}); ok {
			convertedNode.Edges = edges
		}

		convertedNodes = append(convertedNodes, convertedNode)
	}

	return convertedNodes, nil
}

// convertEdges converts edge data following Coze source logic: sourceNodeID→from_node
func (p *CozeParser) convertEdges(jsonData map[string]interface{}) ([]CozeRootEdge, error) {
	var convertedEdges []CozeRootEdge

	edgesData, ok := jsonData["edges"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid edges data format")
	}

	for _, edge := range edgesData {
		edgeMap, ok := edge.(map[string]interface{})
		if !ok {
			continue
		}

		// Following Coze source workflow logic for field mapping
		convertedEdge := CozeRootEdge{
			FromNode: p.getStringValue(edgeMap, "sourceNodeID"),
			FromPort: p.getStringValue(edgeMap, "sourcePortID"),
			ToNode:   p.getStringValue(edgeMap, "targetNodeID"),
			ToPort:   p.getStringValue(edgeMap, "targetPortID"),
		}

		convertedEdges = append(convertedEdges, convertedEdge)
	}

	return convertedEdges, nil
}

// getStringValue safely gets string value following Coze source logic
func (p *CozeParser) getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// buildSchema builds Schema structure for internal compatibility
func (p *CozeParser) buildSchema(jsonData map[string]interface{}) CozeSchema {
	schema := CozeSchema{}

	// Build schema edges in original format
	if edgesData, ok := jsonData["edges"].([]interface{}); ok {
		var schemaEdges []CozeSchemaEdge
		for _, edge := range edgesData {
			if edgeMap, ok := edge.(map[string]interface{}); ok {
				schemaEdge := CozeSchemaEdge{}
				if sourceNodeID, ok := edgeMap["sourceNodeID"].(string); ok {
					schemaEdge.SourceNodeID = sourceNodeID
				}
				if sourcePortID, ok := edgeMap["sourcePortID"].(string); ok {
					schemaEdge.SourcePortID = sourcePortID
				}
				if targetNodeID, ok := edgeMap["targetNodeID"].(string); ok {
					schemaEdge.TargetNodeID = targetNodeID
				}
				if targetPortID, ok := edgeMap["targetPortID"].(string); ok {
					schemaEdge.TargetPortID = targetPortID
				}
				schemaEdges = append(schemaEdges, schemaEdge)
			}
		}
		schema.Edges = schemaEdges
	}

	// Build schema nodes if needed
	if nodesData, ok := jsonData["nodes"].([]interface{}); ok {
		var schemaNodes []CozeSchemaNode
		for _, node := range nodesData {
			if nodeMap, ok := node.(map[string]interface{}); ok {
				schemaNode := CozeSchemaNode{}
				if id, ok := nodeMap["id"].(string); ok {
					schemaNode.ID = id
				}
				if nodeType, ok := nodeMap["type"].(string); ok {
					schemaNode.Type = nodeType
				}
				// Can add more field mappings as needed
				schemaNodes = append(schemaNodes, schemaNode)
			}
		}
		schema.Nodes = schemaNodes
	}

	return schema
}

// Helper function implementations

func (p *CozeParser) extractNameFromManifest(manifestData map[string]interface{}) string {
	if mainData, ok := manifestData["main"].(map[string]interface{}); ok {
		if name, ok := mainData["name"].(string); ok && name != "" {
			return name
		}
	}
	return "Imported Workflow"
}

func (p *CozeParser) extractDescFromManifest(manifestData map[string]interface{}) string {
	if mainData, ok := manifestData["main"].(map[string]interface{}); ok {
		if desc, ok := mainData["desc"].(string); ok && desc != "" {
			return desc
		}
	}
	return "Workflow imported from ZIP format"
}

func (p *CozeParser) extractVersionFromManifest(manifestData map[string]interface{}) string {
	if version, ok := manifestData["version"].(string); ok && version != "" {
		return version
	}
	return "v1.0.0"
}

func (p *CozeParser) extractNodeDataFromMap(nodeMap map[string]interface{}) CozeNodeData {
	nodeData := CozeNodeData{}

	// Extract meta information
	if dataMap, ok := nodeMap["data"].(map[string]interface{}); ok {
		// Extract title from nodeMeta first
		if nodeMetaMap, ok := dataMap["nodeMeta"].(map[string]interface{}); ok {
			nodeData.Meta = CozeDataMeta{
				Title:       p.getStringFromMap(nodeMetaMap, "title", ""),
				Description: p.getStringFromMap(nodeMetaMap, "description", ""),
				Icon:        p.getStringFromMap(nodeMetaMap, "icon", ""),
				Subtitle:    p.getStringFromMap(nodeMetaMap, "subTitle", ""),
			}
		} else if metaMap, ok := dataMap["meta"].(map[string]interface{}); ok {
			// Alternative: extract from meta
			nodeData.Meta = CozeDataMeta{
				Title:       p.getStringFromMap(metaMap, "title", ""),
				Description: p.getStringFromMap(metaMap, "description", ""),
				Icon:        p.getStringFromMap(metaMap, "icon", ""),
				Subtitle:    p.getStringFromMap(metaMap, "subTitle", ""),
			}
		}

		// If no title in meta, use node type as default title
		if nodeData.Meta.Title == "" {
			if nodeType, ok := nodeMap["type"].(string); ok {
				nodeData.Meta.Title = fmt.Sprintf("Node_%s", nodeType)
			} else {
				nodeData.Meta.Title = "Untitled Node"
			}
		}

		// Extract outputs
		if outputs, ok := dataMap["outputs"].([]interface{}); ok {
			nodeData.Outputs = p.convertOutputs(outputs)
		}

		// Create empty inputs structure to avoid nil pointer
		nodeData.Inputs = &CozeNodeInputs{
			InputParameters:    []CozeNodeInputParam{},
			InputParametersAlt: []CozeNodeInputParam{},
			Branches:           []interface{}{},
		}

		// Try to extract inputs data
		if inputs, ok := dataMap["inputs"]; ok {
			p.extractInputsData(inputs, nodeData.Inputs)
		}

	} else {
		// If no data field, use default title and empty inputs
		if nodeType, ok := nodeMap["type"].(string); ok {
			nodeData.Meta.Title = fmt.Sprintf("Node_%s", nodeType)
		} else {
			nodeData.Meta.Title = "Untitled Node"
		}

		// Create empty inputs structure
		nodeData.Inputs = &CozeNodeInputs{
			InputParameters:    []CozeNodeInputParam{},
			InputParametersAlt: []CozeNodeInputParam{},
			Branches:           []interface{}{},
		}
	}

	return nodeData
}

// extractInputsData extracts inputs data following Coze source extractNodeData logic
func (p *CozeParser) extractInputsData(inputs interface{}, nodeInputs *CozeNodeInputs) {
	// Following Coze source logic: preserve original inputs data directly
	if inputsMap, ok := inputs.(map[string]interface{}); ok {

		// Preserve key LLM parameters following Coze source logic
		if llmParam, exists := inputsMap["llmParam"]; exists {
			nodeInputs.LLMParam = llmParam
		}

		// Preserve other important parameters
		if settingOnError, exists := inputsMap["settingOnError"]; exists {
			nodeInputs.SettingOnError = settingOnError
		}

		if intents, exists := inputsMap["intents"]; exists {
			// Ensure intents data format is correct for classifier nodes
			if intentsMap, ok := intents.(map[string]interface{}); ok {
				nodeInputs.IntentDetector = intentsMap
			} else if intentsList, ok := intents.([]interface{}); ok {
				// If array format, convert to map format
				intentsMap := make(map[string]interface{})
				intentsMap["intents"] = intentsList
				nodeInputs.IntentDetector = intentsMap
			} else {
				// If format not recognized, create default map structure
				intentsMap := make(map[string]interface{})
				intentsMap["raw"] = intents
				nodeInputs.IntentDetector = intentsMap
			}
		}

		if code, exists := inputsMap["code"]; exists {
			// Ensure code data format is correct for code nodes
			if codeMap, ok := code.(map[string]interface{}); ok {
				nodeInputs.CodeRunner = codeMap
			} else if codeStr, ok := code.(string); ok {
				// If string format, wrap as map
				codeMap := make(map[string]interface{})
				codeMap["code"] = codeStr
				// Add other necessary code node fields
				if language, exists := inputsMap["language"]; exists {
					codeMap["language"] = language
				} else {
					codeMap["language"] = "python" // Default language
				}
				nodeInputs.CodeRunner = codeMap
			} else {
				// Create default code configuration
				codeMap := make(map[string]interface{})
				codeMap["code"] = ""
				codeMap["language"] = "python"
				nodeInputs.CodeRunner = codeMap
			}
		}

		if inputParams, exists := inputsMap["inputParameters"]; exists {
			if inputParamsList, ok := inputParams.([]interface{}); ok {
				for _, param := range inputParamsList {
					if paramMap, ok := param.(map[string]interface{}); ok {
						nodeParam := CozeNodeInputParam{
							Name: p.getStringFromMap(paramMap, "name", ""),
						}

						// Preserve complete variable reference information
						if inputData, hasInput := paramMap["input"]; hasInput {
							if inputMap, ok := inputData.(map[string]interface{}); ok {
								nodeParam.Input = CozeNodeInput{
									Type: p.getStringFromMap(inputMap, "type", ""),
								}

								// Extract value field variable reference information
								if valueData, hasValue := inputMap["value"]; hasValue {
									if valueMap, ok := valueData.(map[string]interface{}); ok {
										nodeParam.Input.Value = CozeNodeInputValue{
											Type: p.getStringFromMap(valueMap, "type", ""),
										}

										// Extract content field - contains key variable reference information
										if contentData, hasContent := valueMap["content"]; hasContent {
											if contentMap, ok := contentData.(map[string]interface{}); ok {
												nodeParam.Input.Value.Content = CozeNodeInputContent{
													BlockID: p.getStringFromMap(contentMap, "blockID", ""),
													Name:    p.getStringFromMap(contentMap, "name", ""),
													Source:  p.getStringFromMap(contentMap, "source", ""),
												}
											} else if contentStr, ok := contentData.(string); ok {
												// For literal type, content is string, handle appropriately
												// Save string content in special field as temporary solution
												nodeParam.Input.Value.Content = CozeNodeInputContent{
													BlockID: "",
													Name:    contentStr, // Save string content in name field as temporary solution
													Source:  "literal",
												}
											}
										}

										// Extract rawMeta field
										if rawMetaData, hasRawMeta := valueMap["rawMeta"]; hasRawMeta {
											if rawMetaMap, ok := rawMetaData.(map[string]interface{}); ok {
												if rawType, hasType := rawMetaMap["type"]; hasType {
													if typeInt, ok := rawType.(float64); ok {
														nodeParam.Input.Value.RawMeta.Type = int(typeInt)
													}
												}
											}
										}
									}
								}
							}
						}

						nodeInputs.InputParameters = append(nodeInputs.InputParameters, nodeParam)
					}
				}
			}
		}

		// Preserve terminatePlan for end nodes
		if terminatePlan, exists := inputsMap["terminatePlan"]; exists {
			if nodeInputs.Exit == nil {
				nodeInputs.Exit = &CozeExit{}
			}
			if terminatePlanStr, ok := terminatePlan.(string); ok {
				nodeInputs.Exit.TerminatePlan = terminatePlanStr
			}
		}
	}
}

func (p *CozeParser) extractNodeMetaFromMap(nodeMap map[string]interface{}) CozeNodeMeta {
	meta := CozeNodeMeta{}

	if metaMap, ok := nodeMap["meta"].(map[string]interface{}); ok {
		if positionMap, ok := metaMap["position"].(map[string]interface{}); ok {
			position := CozePosition{}
			if x, ok := positionMap["x"].(float64); ok {
				position.X = x
			}
			if y, ok := positionMap["y"].(float64); ok {
				position.Y = y
			}
			meta.Position = position
		}
	}

	return meta
}

func (p *CozeParser) convertOutputs(outputs []interface{}) []CozeOutput {
	var convertedOutputs []CozeOutput

	for _, output := range outputs {
		if outputMap, ok := output.(map[string]interface{}); ok {
			cozeOutput := CozeOutput{
				Name: p.getStringFromMap(outputMap, "name", ""),
				Type: p.getStringFromMap(outputMap, "type", ""),
				Required: func() bool {
					if value, exists := outputMap["required"]; exists {
						if boolVal, ok := value.(bool); ok {
							return boolVal
						}
					}
					return false
				}(),
			}
			if schema, exists := outputMap["schema"]; exists {
				cozeOutput.Schema = schema
			}
			convertedOutputs = append(convertedOutputs, cozeOutput)
		}
	}

	return convertedOutputs
}

func (p *CozeParser) buildMetadata() CozeMetadata {
	return CozeMetadata{
		ContentType: "0",
		CreatorID:   "imported_user",
		Mode:        "0",
		SpaceID:     "imported_space",
	}
}

func (p *CozeParser) buildDependencies(nodes []CozeNode) []CozeDep {
	var dependencies []CozeDep

	for _, node := range nodes {
		nodeTitle := "Node"
		if node.Data.Meta.Title != "" {
			nodeTitle = node.Data.Meta.Title
		}

		dependency := CozeDep{
			Metadata: CozeDepMetadata{
				NodeType: "workflow_node",
			},
			ResourceID:   fmt.Sprintf("node_%s", node.ID),
			ResourceName: nodeTitle,
			ResourceType: "node",
		}
		dependencies = append(dependencies, dependency)
	}

	return dependencies
}

// normalizeNodeStructure converts new format nodes to old format for consistent parsing
func (p *CozeParser) normalizeNodeStructure(node *CozeNode) {
	// If node uses new format (root level title/description/position), convert to old format
	if node.Title != "" && node.Data.Meta.Title == "" {
		// Populate Data.Meta from root level fields
		node.Data.Meta.Title = node.Title
		node.Data.Meta.Description = node.Description
		node.Data.Meta.Icon = node.Icon
	}

	if node.Position != nil && node.Meta.Position.X == 0 && node.Meta.Position.Y == 0 {
		// Populate Meta.Position from root level position
		node.Meta.Position = *node.Position
	}

	// Convert parameters to Data.Outputs and Data.Inputs if needed
	if node.Parameters != nil {
		if paramsMap, ok := node.Parameters.(map[string]interface{}); ok {
			// Handle llmParam for LLM nodes
			if llmParam, exists := paramsMap["llmParam"]; exists {
				if node.Data.Inputs == nil {
					node.Data.Inputs = &CozeNodeInputs{}
				}
				node.Data.Inputs.LLMParam = llmParam
			}

			// Handle fcParamVar for LLM nodes
			if _, exists := paramsMap["fcParamVar"]; exists {
				// fcParamVar contains function calling parameters
				// For now, we'll skip it as it's not critical for basic conversion
			}

			// Handle code and language for code nodes
			if code, exists := paramsMap["code"]; exists {
				if node.Data.Inputs == nil {
					node.Data.Inputs = &CozeNodeInputs{}
				}
				// Store code in CodeRunner structure
				codeRunner := make(map[string]interface{})
				codeRunner["code"] = code

				if language, langExists := paramsMap["language"]; langExists {
					codeRunner["language"] = language
				}

				node.Data.Inputs.CodeRunner = codeRunner
			}

			// Handle node_outputs for start nodes
			if nodeOutputs, exists := paramsMap["node_outputs"]; exists {
				if outputsMap, ok := nodeOutputs.(map[string]interface{}); ok {
					for name, outputData := range outputsMap {
						if outputMap, ok := outputData.(map[string]interface{}); ok {
							outputType := "string"
							if t, exists := outputMap["type"]; exists {
								if typeStr, ok := t.(string); ok {
									outputType = typeStr
								}
							}

							cozeOutput := CozeOutput{
								Name:     name,
								Type:     outputType,
								Required: false,
							}
							node.Data.Outputs = append(node.Data.Outputs, cozeOutput)
						}
					}
				}
			}

			// Handle node_inputs for end nodes and other nodes
			if nodeInputs, exists := paramsMap["node_inputs"]; exists {
				if inputsList, ok := nodeInputs.([]interface{}); ok {
					for _, inputData := range inputsList {
						if inputMap, ok := inputData.(map[string]interface{}); ok {
							name := ""
							if n, exists := inputMap["name"]; exists {
								if nameStr, ok := n.(string); ok {
									name = nameStr
								}
							}

							var inputParam CozeNodeInputParam
							inputParam.Name = name

							if input, exists := inputMap["input"]; exists {
								if inputDetails, ok := input.(map[string]interface{}); ok {
									inputType := "string"
									if t, exists := inputDetails["type"]; exists {
										if typeStr, ok := t.(string); ok {
											inputType = typeStr
										}
									}

									inputParam.Input.Type = inputType

									// Handle value which might contain references
									if value, exists := inputDetails["value"]; exists {
										if valueMap, ok := value.(map[string]interface{}); ok {
											// Check if it's a reference
											if refNode, exists := valueMap["ref_node"]; exists {
												if refNodeStr, ok := refNode.(string); ok {
													inputParam.Input.Value.Type = "ref"
													inputParam.Input.Value.Content.BlockID = refNodeStr

													// Get the path/name
													if path, exists := valueMap["path"]; exists {
														if pathStr, ok := path.(string); ok {
															inputParam.Input.Value.Content.Name = pathStr
														}
													}
												}
											}
										}
									}
								}
							}

							if node.Data.Inputs == nil {
								node.Data.Inputs = &CozeNodeInputs{}
							}
							node.Data.Inputs.InputParameters = append(node.Data.Inputs.InputParameters, inputParam)
						}
					}
				}
			}
		}
	}
}
