// Package cmd
package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/helvethink/gitlab-ci-exporter/internal/collectors"
	"github.com/helvethink/gitlab-ci-exporter/internal/httpServer"
	"github.com/helvethink/gitlab-ci-exporter/internal/logging"
	"github.com/prometheus/common/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type GitlabSettings struct {
	GitlabToken string
	GitlabURL   string
	GitlabUser  string
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "gitlab-ci-exporter",
	Short: "A gitlab CI exporter",
	Long: `The GitLab-CI Exporter for Prometheus is a specialized tool designed to bridge the gap between GitLab's Continuous Integration (CI) pipelines and the Prometheus monitoring system. This application enables users to extract and export vital metrics and data from GitLab CI pipelines, jobs, and other related activities, making them available for monitoring, analysis, and alerting within Prometheus.
Key features of the GitLab-CI Exporter for Prometheus include:
    * Metrics Collection: The exporter gathers comprehensive metrics from GitLab CI, such as job durations, success/failure rates, pipeline execution times, and more. This data is crucial for understanding the performance and health of CI processes.
    * Prometheus Integration: By exposing the collected metrics in a format compatible with Prometheus, the tool allows users to leverage Prometheus's powerful querying language, PromQL, to create custom dashboards and alerts.
    * Real-time Monitoring: Users can monitor their GitLab CI pipelines in real-time, enabling them to quickly identify and address issues, optimize performance, and ensure the reliability of their CI/CD workflows.
    * Customizable Metrics: The exporter can be configured to focus on specific metrics that are most relevant to the user's needs, providing flexibility and customization options.
    * Seamless Setup: Designed with ease of use in mind, the GitLab-CI Exporter for Prometheus can be set up with minimal configuration, allowing users to start monitoring their GitLab CI metrics with little overhead.

Overall, the GitLab-CI Exporter for Prometheus is an essential tool for teams looking to enhance their CI/CD monitoring capabilities, providing valuable insights into their GitLab pipelines and helping to drive continuous improvement in their development processes.`,
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return checkCoreSettings()
	},
	Run: func(cmd *cobra.Command, args []string) {
		exporter()
	},
}

const (
	defaultLogLevel    = "info"
	defaultLogFormat   = "text"
	defaultMetricsPath = "metrics"
	defaultListenPort  = "9101"
	defaultAddress     = "localhost"
	defaultGitlabToken = "need-to-add-something"
	defaultGitlabURL   = "https://gitlab.com"
	defaultGitlabUser  = "gitlab-ci-exporter"
)

var settings collectors.Settings
var gitlabSettings GitlabSettings

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// gochecknoinits:skip
func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	viper.AutomaticEnv()
	viper.SetDefault("LOG_LEVEL", defaultLogLevel)
	viper.SetDefault("LOG_FORMAT", defaultLogFormat)
	viper.SetDefault("METRICS_PATH", defaultMetricsPath)
	viper.SetDefault("LISTEN_PORT", defaultListenPort)
	viper.SetDefault("ADDRESS", defaultAddress)
	viper.SetDefault("GITLAB_TOKEN", defaultGitlabToken)

	rootCmd.Flags().StringVar(&settings.LogLevel, "log.level", defaultLogLevel, "Exporter log level")
	_ = viper.BindPFlag("log.level", rootCmd.Flags().Lookup("LOG_LEVEL"))

	rootCmd.Flags().StringVar(&settings.LogFormat, "log.format", defaultLogFormat, "Exporter log format, text or json")
	_ = viper.BindPFlag("log.format", rootCmd.Flags().Lookup("LOG_FORMAT"))

	rootCmd.Flags().StringVar(&settings.MetricsPath, "metrics.path", defaultMetricsPath, "Path to expose metrics at")
	_ = viper.BindPFlag("metrics.path", rootCmd.Flags().Lookup("METRICS_PATH"))

	rootCmd.Flags().StringVar(&settings.ListenPort, "listen.port", defaultListenPort, "Port to listen at")
	_ = viper.BindPFlag("listen.port", rootCmd.Flags().Lookup("LISTEN_PORT"))

	rootCmd.Flags().StringVar(&settings.Address, "address", defaultAddress, "The address to access the exporter used for oauth redirect uri")
	_ = viper.BindPFlag("address", rootCmd.Flags().Lookup("ADDRESS"))

	rootCmd.Flags().StringVar(&gitlabSettings.GitlabToken, "gitlab.token", defaultGitlabToken, "Gitlab access token")
	_ = viper.BindPFlag("gitlab.token", rootCmd.Flags().Lookup("GITLAB_TOKEN"))

	rootCmd.Flags().StringVar(&gitlabSettings.GitlabURL, "gitlab.url", defaultGitlabURL, "Gitlab instance URL")
	_ = viper.BindPFlag("gitlab.url", rootCmd.Flags().Lookup("GITLAB_URL"))

	rootCmd.Flags().StringVar(&gitlabSettings.GitlabUser, "gitlab.user", defaultGitlabUser, "Gitlab user name")
	_ = viper.BindPFlag("gitlab.user", rootCmd.Flags().Lookup("GITLAB_USER"))

	settings.LogLevel = viper.GetString("LOG_LEVEL")
	settings.LogFormat = viper.GetString("LOG_FORMAT")
	settings.MetricsPath = viper.GetString("METRICS_PATH")
	settings.ListenPort = viper.GetString("LISTEN_PORT")
	settings.Address = viper.GetString("ADDRESS")
	gitlabSettings.GitlabToken = viper.GetString("GITLAB_TOKEN")
	gitlabSettings.GitlabURL = viper.GetString("GITLAB_URL")
	gitlabSettings.GitlabUser = viper.GetString("GITLAB_USER")
}

func checkCoreSettings() error {
	stgs := &settings

	// Check if the settings are correctly sets before starting app
	if stgs.LogLevel == "" {
		return fmt.Errorf("missing Log Level")
	}

	return nil
}

func exporter() {
	s := &settings

	logger, err := logging.NewLogger(s.LogLevel, s.LogFormat)
	if err != nil {
		logger.Warn(err.Error())
	}

	logger.Info(fmt.Sprintf("starting prometheus exporter %v %v", version.Info(), version.BuildContext()))
	exporter, err := collectors.NewExporter(s, logger)
	if err != nil {
		logger.Error("Failed to create GitLab CI Prometheus Exporter", "err", err)
		os.Exit(1)
	}

	srv := httpServer.NewServer(exporter)
	logger.Info(fmt.Sprintf("Starting GitLab CI Prometheus Exporter on port: %v", s.ListenPort))
	log.Fatal(srv.ListenAndServe())
}
