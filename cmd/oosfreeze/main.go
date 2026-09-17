package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/oosfreeze"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var action, configPath, ensemblePath, outPath string
	flag.StringVar(&action, "action", "record", "record or status")
	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&ensemblePath, "ensemble-path", "", "ensemble model JSON (default: model.ensemble_path from config)")
	flag.StringVar(&outPath, "out", "artifacts/oos/freeze.json", "path to the freeze manifest")
	flag.Parse()

	switch action {
	case "record":
		return record(configPath, ensemblePath, outPath)
	case "status":
		return status(configPath, ensemblePath, outPath)
	default:
		return fmt.Errorf("unknown action %q, want record or status", action)
	}
}

func record(configPath, ensemblePath, outPath string) error {
	recipe, err := buildRecipe(configPath, ensemblePath)
	if err != nil {
		return err
	}
	codeSHA, err := gitSHA()
	if err != nil {
		return fmt.Errorf("resolve code SHA: %w", err)
	}
	manifest := oosfreeze.NewManifest(time.Now().UTC(), codeSHA, recipe)
	if err := oosfreeze.WriteManifest(outPath, manifest); err != nil {
		return err
	}
	fmt.Printf("froze OOS recipe at %s\n", manifest.FrozenAt.Format(time.RFC3339))
	fmt.Printf("code SHA: %s\n", manifest.CodeSHA)
	fmt.Printf("recipe SHA256: %s\n", manifest.RecipeHash)
	fmt.Printf("model SHA256: %s\n", manifest.Recipe.ModelSHA256)
	fmt.Printf("config SHA256: %s\n", manifest.Recipe.ConfigSHA256)
	fmt.Printf("manifest: %s\n", outPath)
	return nil
}

func status(configPath, ensemblePath, outPath string) error {
	manifest, err := oosfreeze.LoadManifest(outPath)
	if err != nil {
		return err
	}
	recipe, err := buildRecipe(configPath, ensemblePath)
	if err != nil {
		return err
	}
	check := oosfreeze.Check(manifest, recipe, time.Now())
	fmt.Printf("frozen at: %s\n", manifest.FrozenAt.Format(time.RFC3339))
	fmt.Printf("recorded code SHA: %s\n", manifest.CodeSHA)
	fmt.Printf("recipe unchanged: %v\n", !check.Changed)
	fmt.Printf("collection days: %d / %d\n", check.CollectionDays, check.RequiredDays)
	fmt.Printf("minimum collection complete: %v\n", check.Complete)
	fmt.Printf("real-money execution enabled: %v\n", manifest.RealMoneyEnabled)
	if currentCode, err := gitSHA(); err == nil && currentCode != manifest.CodeSHA {
		fmt.Printf("current code SHA: %s (informational, does not restart the OOS count)\n", currentCode)
	}
	if check.Changed {
		fmt.Println("recipe changed; the OOS count is zero and a new freeze must be recorded")
	}
	return nil
}

func buildRecipe(configPath, ensemblePath string) (oosfreeze.Recipe, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return oosfreeze.Recipe{}, fmt.Errorf("load config: %w", err)
	}
	if ensemblePath == "" {
		ensemblePath = cfg.Model.EnsemblePath
	}
	if ensemblePath == "" {
		ensemblePath = "ensemble_model.json"
	}
	modelBytes, err := os.ReadFile(ensemblePath)
	if err != nil {
		return oosfreeze.Recipe{}, fmt.Errorf("read ensemble model: %w", err)
	}
	ensemble, err := model.LoadEnsembleModel(ensemblePath)
	if err != nil {
		return oosfreeze.Recipe{}, fmt.Errorf("load ensemble model: %w", err)
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return oosfreeze.Recipe{}, fmt.Errorf("read config: %w", err)
	}
	return oosfreeze.Recipe{
		ModelSHA256:              oosfreeze.HashBytes(modelBytes),
		ConfigSHA256:             oosfreeze.HashBytes(configBytes),
		FeatureOrder:             append([]string(nil), ensemble.FeatureOrder...),
		BuyThreshold:             ensemble.BuyThreshold,
		SellThreshold:            ensemble.SellThreshold,
		Tickers:                  append([]string(nil), cfg.Tickers...),
		EnsemblePath:             ensemblePath,
		TargetNotional:           cfg.Risk.TargetNotional.String(),
		CommissionRate:           cfg.Commission.Rate.String(),
		MaxSlippagePct:           cfg.Risk.MaxSlippagePct.String(),
		RebalanceMinDeviationPct: cfg.Risk.RebalanceMinDeviationPct.String(),
		NoTradeAfterOpenMinutes:  cfg.Risk.NoTradeAfterOpenMinutes,
		BlackoutWindows:          append([]string{}, cfg.Risk.BlackoutWindows...),
		PollInterval:             cfg.PollInterval.Std().String(),
		OrderType:                cfg.Tinkoff.OrderType,
		Broker:                   cfg.Broker,
		IsPaperTrading:           cfg.IsPaperTrading,
	}, nil
}

func gitSHA() (string, error) {
	output, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
