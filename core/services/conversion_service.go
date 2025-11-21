package services

import (
	"context"
	"fmt"

	"github.com/iflytek/agentbridge/core/interfaces"
	"github.com/iflytek/agentbridge/internal/models"
	"github.com/iflytek/agentbridge/platforms/common"
)

// ConversionService orchestrates DSL conversion between platforms.
type ConversionService struct {
	strategyRegistry StrategyRegistry
}

// validationDisabled controls whether unified DSL validation runs during conversion.
var validationDisabled = true

// NewConversionService creates a conversion service with the provided strategy registry.
func NewConversionService(registry StrategyRegistry) *ConversionService {
	return &ConversionService{
		strategyRegistry: registry,
	}
}

// Convert performs DSL conversion from source to target format.
func (s *ConversionService) Convert(
	sourceData []byte,
	sourcePlatform, targetPlatform models.PlatformType,
) ([]byte, error) {
	return s.ConvertWithContext(context.Background(), sourceData, sourcePlatform, targetPlatform)
}

// ConvertWithContext performs DSL conversion with request context support.
func (s *ConversionService) ConvertWithContext(
	ctx context.Context,
	sourceData []byte,
	sourcePlatform, targetPlatform models.PlatformType,
) ([]byte, error) {
	// Check platform support
	if err := s.validatePlatformSupport(sourcePlatform, targetPlatform); err != nil {
		return nil, &models.ConversionError{
			Code:           "PLATFORM_NOT_SUPPORTED",
			Message:        "Platform validation failed",
			SourcePlatform: string(sourcePlatform),
			TargetPlatform: string(targetPlatform),
			ErrorType:      "platform_support",
			Details:        err.Error(),
			Severity:       models.SeverityCritical,
		}
	}

	// Get source platform parser
	parser, err := s.getParser(sourcePlatform)
	if err != nil {
		return nil, &models.ConversionError{
			Code:           "PARSER_NOT_FOUND",
			Message:        fmt.Sprintf("Failed to get parser for %s", sourcePlatform),
			SourcePlatform: string(sourcePlatform),
			TargetPlatform: string(targetPlatform),
			ErrorType:      "parser_error",
			Details:        err.Error(),
			Severity:       models.SeverityCritical,
		}
	}

	// Parse source DSL to unified format
	unifiedDSL, err := parser.Parse(sourceData)
	if err != nil {
		return nil, &models.ParseError{
			Code:    "PARSE_FAILED",
			Message: "Failed to parse source DSL",
			Suggestions: []string{
				"Check DSL format and syntax",
				"Verify all required fields are present",
				"Ensure file encoding is correct",
			},
		}
	}

	// Basic validation using the common validator
	if err := s.performValidation(unifiedDSL); err != nil {
		return nil, err // Already a typed error
	}

	// Get target platform generator
	generator, err := s.getGenerator(targetPlatform)
	if err != nil {
		return nil, &models.ConversionError{
			Code:           "GENERATOR_NOT_FOUND",
			Message:        fmt.Sprintf("Failed to get generator for %s", targetPlatform),
			SourcePlatform: string(sourcePlatform),
			TargetPlatform: string(targetPlatform),
			ErrorType:      "generator_error",
			Details:        err.Error(),
			Severity:       models.SeverityCritical,
		}
	}

	// Generate target platform DSL
	targetData, err := generator.Generate(unifiedDSL)
	if err != nil {
		return nil, &models.ConversionError{
			Code:           "GENERATION_FAILED",
			Message:        "Failed to generate target DSL",
			SourcePlatform: string(sourcePlatform),
			TargetPlatform: string(targetPlatform),
			ErrorType:      "generation_error",
			Details:        err.Error(),
			Severity:       models.SeverityError,
			Suggestions: []string{
				"Check if all nodes are supported on target platform",
				"Verify node configurations are valid",
				"Review variable references and connections",
			},
		}
	}

	return targetData, nil
}

// validatePlatformSupport checks if the source and target platforms are supported.
func (s *ConversionService) validatePlatformSupport(sourcePlatform, targetPlatform models.PlatformType) error {
	supportedPlatforms := s.strategyRegistry.GetSupportedPlatforms()

	// Check source platform support
	sourceSupported := false
	targetSupported := false

	for _, platform := range supportedPlatforms {
		if platform == sourcePlatform {
			sourceSupported = true
		}
		if platform == targetPlatform {
			targetSupported = true
		}
	}

	if !sourceSupported {
		return fmt.Errorf("source platform %s is not supported", sourcePlatform)
	}

	if !targetSupported {
		return fmt.Errorf("target platform %s is not supported", targetPlatform)
	}

	return nil
}

// performValidation performs comprehensive validation using the common validator
func (s *ConversionService) performValidation(unifiedDSL *models.UnifiedDSL) error {
	if validationDisabled {
		return nil
	}

	// Use the common DSL validator
	validator := common.NewUnifiedDSLValidator()

	// Validate metadata
	if err := validator.ValidateMetadata(&unifiedDSL.Metadata); err != nil {
		return &models.ValidationError{
			Type:           "metadata",
			Severity:       "error",
			Message:        fmt.Sprintf("Metadata validation failed: %v", err),
			AffectedItems:  []string{err.Error()},
			FixSuggestions: []string{"Check workflow name and description", "Verify metadata completeness"},
		}
	}

	// Validate workflow
	if err := validator.ValidateWorkflow(&unifiedDSL.Workflow); err != nil {
		return &models.ValidationError{
			Type:           "workflow",
			Severity:       "error",
			Message:        fmt.Sprintf("Workflow validation failed: %v", err),
			AffectedItems:  []string{err.Error()},
			FixSuggestions: []string{"Check node connections", "Verify required start and end nodes", "Validate node references"},
		}
	}

	return nil
}

// ValidateSourceData validates input data against source platform requirements.
func (s *ConversionService) ValidateSourceData(
	data []byte,
	platform models.PlatformType,
) error {
	parser, err := s.getParser(platform)
	if err != nil {
		return fmt.Errorf("failed to get parser for validation: %w", err)
	}

	return parser.Validate(data)
}

// ValidateTargetCompatibility verifies unified DSL compatibility with target platform.
func (s *ConversionService) ValidateTargetCompatibility(
	unifiedDSL *models.UnifiedDSL,
	platform models.PlatformType,
) error {
	generator, err := s.getGenerator(platform)
	if err != nil {
		return fmt.Errorf("failed to get generator for validation: %w", err)
	}

	return generator.Validate(unifiedDSL)
}

func (s *ConversionService) getParser(platform models.PlatformType) (interfaces.DSLParser, error) {
	strategy, err := s.strategyRegistry.GetStrategy(platform)
	if err != nil {
		return nil, err
	}

	return strategy.CreateParser()
}

func (s *ConversionService) getGenerator(platform models.PlatformType) (interfaces.DSLGenerator, error) {
	strategy, err := s.strategyRegistry.GetStrategy(platform)
	if err != nil {
		return nil, err
	}

	return strategy.CreateGenerator()
}

// StrategyRegistry manages platform-specific conversion strategies.
type StrategyRegistry interface {
	// GetStrategy retrieves the strategy for the specified platform
	GetStrategy(platform models.PlatformType) (PlatformStrategy, error)

	// RegisterStrategy registers a strategy for a platform
	RegisterStrategy(platform models.PlatformType, strategy PlatformStrategy)

	// GetSupportedPlatforms returns list of supported platforms
	GetSupportedPlatforms() []models.PlatformType
}

// PlatformStrategy defines the interface for platform-specific conversion strategies.
// Simplified to include only the methods that are actually used.
type PlatformStrategy interface {
	// GetPlatformType returns the platform identifier
	GetPlatformType() models.PlatformType

	// CreateParser creates a DSL parser for this platform
	CreateParser() (interfaces.DSLParser, error)

	// CreateGenerator creates a DSL generator for this platform
	CreateGenerator() (interfaces.DSLGenerator, error)
}
