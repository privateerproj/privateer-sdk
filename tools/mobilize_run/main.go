package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/revanite-io/pvtr-github-repo/evaluation_plans"
)

//go:embed ../..//revanite/pvtr-github-repo/data/catalogs
var files embed.FS

func main() {
	orchestrator := pluginkit.EvaluationOrchestrator{
		PluginName: "github-repo",
	}

	dataDir := filepath.Join("..", "..", "revanite", "pvtr-github-repo", "data", "catalogs")
	err := orchestrator.AddReferenceCatalogs(dataDir, files)
	if err != nil {
		fmt.Printf("Error loading catalogs: %v\n", err)
		os.Exit(1)
	}

	orchestrator.AddEvaluationSuite("osps-baseline", nil, evaluation_plans.OSPS)
	orchestrator.AddRequiredVars([]string{})

	err = orchestrator.Mobilize()
	if err != nil {
		fmt.Printf("Mobilize error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Mobilize completed")
}
