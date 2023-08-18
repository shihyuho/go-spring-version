package main

import (
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"github.com/creekorful/mvnparser"
	"github.com/spf13/cobra"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var (
	// The list of supported BOM names can be found in the 'initializr.env.boms' section of the
	// application.yml file located at: https://github.com/spring-io/start.spring.io/blob/main/start-site/src/main/resources/application.yml
	supportedBoms = []string{
		"spring-cloud",
		"spring-cloud-azure",
		"spring-cloud-gcp",
		"spring-cloud-services",
		"spring-modulith",
		"spring-shell",
		"codecentric-spring-boot-admin",
		"hilla",
		"sentry",
		"solace-spring-boot",
		"solace-spring-cloud",
		"testcontainers",
		"vaadin",
		"wavefront",
	}
)

const (
	outputStdout      = "stdout"
	outputGithub      = "github"
	defaultStarterURL = "https://start.spring.io"
	defaultBootURL    = "https://api.spring.io/projects/spring-boot/releases"
	defaultTypeID     = "maven-build"
	desc              = `This command get the latest Spring Boot version and its associated BOM versions, e.g. Spring Cloud.

You can specify the '-b, --boot-version' flag to determine the Spring Boot version,
or you can leave it blank to use the current version.
Furthermore, '-b' flag also supports Semantic Versioning (semver) version comparison.

  $ spring-version
  $ spring-version -b 2.7.15-SNAPSHOT
  $ spring-version -b ">=2.0.0, <4.0.0"
  $ spring-version -b ~3.x

You can also use the '-d, --dependency' flag multiple times to specify dependencies.
Alternatively, you can pass dependencies by separating them with commas, e.g. foo, bar.

  $ spring-version -d cloud-starter -d native
  $ spring-version -d cloud-starter,devtools -d native

You can use the '--starter-url' flag to define the URL of the starter metadata server,
and you can also utilize the '--boot-url' flag to establish the URL for the Spring Boot metadata server.

  $ spring-version --starter-url https://mystarter.com:8080
`
)

type Config struct {
	Metadata     Metadata
	BootVersion  string
	TypeID       string
	Dependencies []string
	Output       string
	Verbose      bool
}

type Metadata struct {
	StarterURL string
	BootURL    string
	Insecure   bool
}

type StarterMetadata struct {
	Type struct {
		Type    string `json:"type"`
		Default string `json:"default"`
		Values  []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Action      string `json:"action"`
			Tags        struct {
				Build   string `json:"build"`
				Dialect string `json:"dialect"`
				Format  string `json:"format"`
			} `json:"tags"`
		} `json:"values"`
	} `json:"type"`
}

type BootMetadata struct {
	Embedded struct {
		Releases []struct {
			Version         string `json:"version"`
			APIDocURL       string `json:"apiDocUrl"`
			ReferenceDocURL string `json:"referenceDocUrl"`
			Status          string `json:"status"`
			Current         bool   `json:"current"`
			Links           struct {
				Repository struct {
					Href string `json:"href"`
				} `json:"repository"`
				Self struct {
					Href string `json:"href"`
				} `json:"self"`
			} `json:"_links"`
		} `json:"releases"`
	} `json:"_embedded"`
}

func main() {
	c := Config{
		Metadata: Metadata{
			StarterURL: defaultStarterURL,
			BootURL:    defaultBootURL,
		},
		TypeID: defaultTypeID,
		Output: outputStdout,
	}

	cmd := &cobra.Command{
		Use:          "spring-version",
		Short:        "Get the latest Spring version",
		Long:         desc,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(c)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&c.Metadata.StarterURL, "starter-url", c.Metadata.StarterURL, "URL of Starter metadata")
	flags.StringVar(&c.Metadata.BootURL, "boot-url", c.Metadata.BootURL, "URL of Spring Boot metadata")
	flags.BoolVarP(&c.Metadata.Insecure, "insecure", "k", c.Metadata.Insecure, "Allow insecure metadata server connections when using SSL")
	flags.StringVar(&c.TypeID, "type-id", c.TypeID, "Type ID of the action in Spring Boot metadata")
	flags.StringVarP(&c.BootVersion, "boot-version", "b", c.BootVersion, "Spring Boot version, supports semver comparison")
	flags.StringSliceVarP(&c.Dependencies, "dependency", "d", c.Dependencies, "List of dependency identifiers to include in the generated project")
	flags.StringVarP(&c.Output, "output", "o", c.Output, "Output destination, where to write the result. Options: "+outputStdout+", "+outputGithub)
	flags.BoolVarP(&c.Verbose, "verbose", "v", c.Verbose, "enable verbose output")
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// run is the entrypoint
func run(c Config) (err error) {
	fmt.Printf("Fetching Spring Boot Metadata from %s\n", c.Metadata.BootURL)
	var boot BootMetadata
	if err = fetchMetadata(c.Metadata.BootURL, c.Metadata.Insecure, &boot); err != nil {
		return err
	}
	if c.BootVersion, err = boot.determineBootVersion(c.BootVersion); err != nil {
		return err
	}
	fmt.Printf("Fetching Starter Metadata from %s\n", c.Metadata.StarterURL)
	var starter StarterMetadata
	if err = fetchMetadata(c.Metadata.StarterURL, c.Metadata.Insecure, &starter); err != nil {
		return err
	}
	var action string
	if action, err = starter.getAction(c.TypeID); err != nil {
		return err
	}
	project, err := c.loadMavenProject(action)
	if err != nil {
		return err
	}
	if err = writeln(c.Output, "spring-boot="+c.BootVersion); err != nil {
		return err
	}
	metadata := make(map[string]interface{})
	for k, v := range project.Properties {
		metadata[k] = v
		if bom := firstMatchingPrefix(k, supportedBoms...); bom != "" {
			if err = writef(c.Output, "%s=%s\n", bom, v); err != nil {
				return err
			}
		}
	}
	if c.Verbose {
		if m, err := json.Marshal(metadata); err == nil {
			if err = writef(c.Output, "metadata=%s\n", m); err != nil {
				return err
			}
		}
	}
	return nil
}

// firstMatchingPrefix returns the first prefix that matches the given string.
func firstMatchingPrefix(s string, prefixes ...string) string {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return prefix
		}
	}
	return ""
}

// fetchMetadata fetched Metadata and convert it to v
func fetchMetadata(api string, insecure bool, v any) error {
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}}
	response, err := client.Get(api)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, &v)
}

// determineBootVersion decides the Spring Boot version
func (b *BootMetadata) determineBootVersion(target string) (string, error) {
	if target == "" {
		target = b.findCurrentReleaseVersion()
	}
	if target == "" {
		return "", errors.New("can not determine spring-boot version")
	}
	versions, err := b.collectVersionCandidates()
	if err != nil {
		return "", err
	}
	constraints, err := semver.NewConstraint(target)
	if err != nil {
		return "", fmt.Errorf("invalid spring-boot constraint format: %s: %w", target, err)
	}
	target, err = selectLatestMatchingVersion(versions, constraints)
	if err != nil {
		return "", err
	}
	fmt.Println("Final selected spring-boot version:", target)
	return target, nil
}

// findCurrentReleaseVersion searches for the current release version
func (b *BootMetadata) findCurrentReleaseVersion() string {
	for _, r := range b.Embedded.Releases {
		if r.Current {
			return r.Version
		}
	}
	return ""
}

// collectVersionCandidates collect spring-boot version candidates
func (b *BootMetadata) collectVersionCandidates() (versions []string, err error) {
	for _, release := range b.Embedded.Releases {
		versions = append(versions, release.Version)
	}
	if len(versions) == 0 {
		return nil, errors.New("no valid spring-boot versions found")
	}
	return versions, nil
}

// selectLatestMatchingVersion select the latest version matches the given constraints
func selectLatestMatchingVersion(versions []string, constraints *semver.Constraints) (string, error) {
	var selected *semver.Version
	for _, ver := range versions {
		v, err := semver.NewVersion(ver)
		if err != nil {
			return "", fmt.Errorf("invalid spring-boot version format: %s: %w", ver, err)
		}
		if constraints.Check(v) {
			if selected == nil || v.GreaterThan(selected) {
				selected = v
			}
		}
	}
	if selected != nil {
		return selected.Original(), nil
	}
	return "", fmt.Errorf("no spring-boot version matching the given constraints [%s]: %s\n", constraints.String(), strings.Join(versions, ", "))
}

// getAction retrieves the action corresponding to the targetID from the starter
func (s *StarterMetadata) getAction(targetID string) (string, error) {
	for _, t := range s.Type.Values {
		if t.ID == targetID {
			return t.Action, nil
		}
	}
	return "", errors.New("can not determine type action")
}

// loadMavenProject loads a Maven project from the starter
func (c *Config) loadMavenProject(action string) (project *mvnparser.MavenProject, err error) {
	queryParams := url.Values{
		"BootVersion":  []string{c.BootVersion},
		"dependencies": c.flatDependencies(),
	}
	response, err := http.Get(fmt.Sprintf("%s%s?%s", c.Metadata.StarterURL, action, queryParams.Encode()))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	err = xml.Unmarshal(body, &project)
	return project, err
}

// flatDependencies flattens the dependencies containing commas
func (c *Config) flatDependencies() (dependencies []string) {
	for _, item := range c.Dependencies {
		parts := strings.Split(item, ",")
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				dependencies = append(dependencies, trimmed)
			}
		}
	}
	return dependencies
}

// writeln writes the text to the output and adds a line break
func writeln(output, text string) error {
	return writef(output, "%s\n", text)
}

// writef formats the text and then writes it to the output
func writef(output, format string, a ...any) error {
	return write(output, fmt.Sprintf(format, a...))
}

// write writes the text to the output
func write(output, text string) (err error) {
	var out *os.File
	switch output {
	case outputStdout:
		out = os.Stdout
	case outputGithub:
		path := os.Getenv("GITHUB_OUTPUT")
		if path == "" {
			return fmt.Errorf("environment variable GITHUB_OUTPUT must be set")
		}
		out, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("could not open github output file for writing: %w", err)
		}
		defer out.Close()
	default:
		return fmt.Errorf("unsupported output type: %s", output)
	}
	_, err = out.WriteString(text)
	return err
}
