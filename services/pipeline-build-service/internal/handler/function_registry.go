package handler

import (
	"context"
	"strings"

	pipelineexpression "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
)

type PipelineFunctionRuntime string

const (
	PipelineFunctionRuntimeExpression PipelineFunctionRuntime = "expression"
	PipelineFunctionRuntimePython     PipelineFunctionRuntime = "python"
)

type PipelineFunctionParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type PipelineFunctionDefinition struct {
	ID          string                      `json:"id"`
	Name        string                      `json:"name"`
	DisplayName string                      `json:"display_name,omitempty"`
	Description string                      `json:"description,omitempty"`
	Version     string                      `json:"version"`
	Runtime     PipelineFunctionRuntime     `json:"runtime"`
	Parameters  []PipelineFunctionParameter `json:"parameters,omitempty"`
	ResultType  string                      `json:"result_type,omitempty"`
	Expression  string                      `json:"expression,omitempty"`
	Source      string                      `json:"source,omitempty"`
	Entrypoint  string                      `json:"entrypoint,omitempty"`
	Docs        []string                    `json:"docs,omitempty"`
}

type PipelineFunctionRef struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	AutoUpgrade bool   `json:"auto_upgrade,omitempty"`
}

// PipelineFunctionRegistry is deliberately small: production can adapt it to
// Ontology Functions later, while tests can inject deterministic versioned UDFs.
type PipelineFunctionRegistry interface {
	ListPipelineFunctions(ctx context.Context) ([]PipelineFunctionDefinition, error)
	ResolvePipelineFunction(ctx context.Context, ref PipelineFunctionRef) (PipelineFunctionDefinition, error)
}

func normalisePipelineFunction(def PipelineFunctionDefinition) PipelineFunctionDefinition {
	def.ID = strings.TrimSpace(def.ID)
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		def.Name = def.ID
	}
	def.DisplayName = strings.TrimSpace(def.DisplayName)
	if def.DisplayName == "" {
		def.DisplayName = def.Name
	}
	def.Version = strings.TrimSpace(def.Version)
	if def.Version == "" {
		def.Version = "0.1.0"
	}
	switch PipelineFunctionRuntime(strings.ToLower(strings.TrimSpace(string(def.Runtime)))) {
	case PipelineFunctionRuntimePython:
		def.Runtime = PipelineFunctionRuntimePython
	default:
		def.Runtime = PipelineFunctionRuntimeExpression
	}
	if strings.TrimSpace(def.ResultType) == "" {
		def.ResultType = "String"
	}
	for i := range def.Parameters {
		def.Parameters[i].Name = strings.TrimSpace(def.Parameters[i].Name)
		if strings.TrimSpace(def.Parameters[i].Type) == "" {
			def.Parameters[i].Type = "String"
		}
	}
	return def
}

func pipelineFunctionType(raw string) pipelineexpression.PipelineType {
	ty, ok := pipelineexpression.ParseTypeLiteral(strings.ToUpper(strings.TrimSpace(raw)))
	if ok {
		return ty
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "bool":
		return pipelineexpression.BooleanType()
	case "int":
		return pipelineexpression.IntegerType()
	case "float":
		return pipelineexpression.DoubleType()
	}
	return pipelineexpression.StringType()
}
