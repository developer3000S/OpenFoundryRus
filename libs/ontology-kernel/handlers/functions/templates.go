// Static data for `GetFunctionAuthoringSurface`. Mirrors the Rust
// `built_in_function_authoring_templates()` and `function_sdk_packages()`
// helpers — three templates (TS search companion, TS governed mutation,
// Python analysis kit) + three SDK pointers + four CLI commands.
package functions

import (
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
)

const tsSearchCompanionStarter = `export default async function handler(context) {
  const target = context.targetObject;
  const related = await context.sdk.ontology.search({
    query: target?.properties?.name ?? 'high risk case',
    kind: 'object_instance',
    limit: 5,
  });

  const summary = context.capabilities.allowAi
    ? await context.llm.complete({
        userMessage: ` + "`Summarize the current operating posture for ${target?.id ?? 'this selection'}.`" + `,
        maxTokens: 160,
      })
    : null;

  return {
    output: {
      inspectedObjectId: target?.id ?? null,
      related,
      summary: summary?.reply ?? null,
    },
  };
}`

const tsGovernedMutationStarter = `export default async function handler(context) {
  const target = context.targetObject;

  return {
    output: {
      targetObjectId: target?.id ?? null,
      decidedStatus: 'reviewed',
    },
    object_patch: target
      ? {
          status: 'reviewed',
          review_note: context.parameters.payload?.note ?? 'Reviewed by governed function',
        }
      : null,
  };
}`

const pythonAnalysisKitStarter = `def handler(context):
    target = context.get("target_object")
    related = context["sdk"].ontology.search(
        query=(target or {}).get("properties", {}).get("name", "high risk case"),
        kind="object_instance",
        limit=5,
    )

    summary = None
    if context["capabilities"].get("allow_ai"):
        summary = context["llm"].complete(
            user_message=f"Summarize object {(target or {}).get('id', 'n/a')} in one sentence.",
            max_tokens=128,
        )

    return {
        "output": {
            "inspectedObjectId": (target or {}).get("id"),
            "related": related,
            "summary": summary,
        }
    }`

func builtInFunctionAuthoringTemplates() []models.FunctionAuthoringTemplate {
	tsScaffold := "function-typescript"
	pyScaffold := "function-python"
	return []models.FunctionAuthoringTemplate{
		{
			ID:            "typescript-search-companion",
			Runtime:       "typescript",
			DisplayName:   "TypeScript search companion",
			Description:   "Read ontology context, query related objects, and optionally summarize results with the LLM.",
			Entrypoint:    "default",
			StarterSource: tsSearchCompanionStarter,
			DefaultCapabilities: models.FunctionCapabilities{
				AllowOntologyRead:  true,
				AllowOntologyWrite: false,
				AllowAI:            true,
				AllowNetwork:       false,
				TimeoutSeconds:     15,
				MaxSourceBytes:     65_536,
			},
			RecommendedUseCases: []string{
				"semantic retrieval",
				"read-only copilots",
				"case summarization",
			},
			CLIScaffoldTemplate: &tsScaffold,
			SDKPackages: []string{
				"@open-foundry/sdk",
				"@open-foundry/sdk/react",
			},
		},
		{
			ID:            "typescript-governed-mutation",
			Runtime:       "typescript",
			DisplayName:   "TypeScript governed mutation",
			Description:   "Return structured ontology effects such as object patches or link instructions behind an action.",
			Entrypoint:    "default",
			StarterSource: tsGovernedMutationStarter,
			DefaultCapabilities: models.FunctionCapabilities{
				AllowOntologyRead:  true,
				AllowOntologyWrite: true,
				AllowAI:            false,
				AllowNetwork:       false,
				TimeoutSeconds:     15,
				MaxSourceBytes:     65_536,
			},
			RecommendedUseCases: []string{
				"action-backed edits",
				"governed object patches",
				"decision orchestration",
			},
			CLIScaffoldTemplate: &tsScaffold,
			SDKPackages: []string{
				"@open-foundry/sdk",
				"@open-foundry/sdk/react",
			},
		},
		{
			ID:            "python-analysis-kit",
			Runtime:       "python",
			DisplayName:   "Python analysis kit",
			Description:   "Use the Python runtime for object inspection, lightweight calculations, and controlled AI-assisted analysis.",
			Entrypoint:    "handler",
			StarterSource: pythonAnalysisKitStarter,
			DefaultCapabilities: models.FunctionCapabilities{
				AllowOntologyRead:  true,
				AllowOntologyWrite: false,
				AllowAI:            true,
				AllowNetwork:       false,
				TimeoutSeconds:     15,
				MaxSourceBytes:     65_536,
			},
			RecommendedUseCases: []string{
				"python-native analysis",
				"operational calculators",
				"AI-assisted diagnostics",
			},
			CLIScaffoldTemplate: &pyScaffold,
			SDKPackages: []string{"openfoundry-sdk"},
		},
	}
}

func functionSDKPackages() []models.FunctionSDKPackageReference {
	return []models.FunctionSDKPackageReference{
		{
			Language:    "typescript",
			Path:        "sdks/typescript/openfoundry-sdk",
			PackageName: "@open-foundry/sdk",
			GeneratedBy: "go run ./tools/of-cli docs generate-sdk-typescript --input apps/web/public/generated/openapi/openfoundry.json --output sdks/typescript/openfoundry-sdk",
		},
		{
			Language:    "python",
			Path:        "sdks/python/openfoundry-sdk",
			PackageName: "openfoundry-sdk",
			GeneratedBy: "go run ./tools/of-cli docs generate-sdk-python --input apps/web/public/generated/openapi/openfoundry.json --output sdks/python/openfoundry-sdk",
		},
		{
			Language:    "java",
			Path:        "sdks/java/openfoundry-sdk",
			PackageName: "com.openfoundry.sdk",
			GeneratedBy: "go run ./tools/of-cli docs generate-sdk-java --input apps/web/public/generated/openapi/openfoundry.json --output sdks/java/openfoundry-sdk",
		},
	}
}

func functionAuthoringCLICommands() []string {
	return []string{
		"go run ./tools/of-cli project init customer-triage --template function-typescript --output packages",
		"go run ./tools/of-cli project init anomaly-diagnostics --template function-python --output packages",
		"go run ./tools/of-cli docs generate-sdk-typescript --input apps/web/public/generated/openapi/openfoundry.json --output sdks/typescript/openfoundry-sdk",
		"go run ./tools/of-cli docs generate-sdk-python --input apps/web/public/generated/openapi/openfoundry.json --output sdks/python/openfoundry-sdk",
	}
}
