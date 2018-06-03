package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
	"github.com/bitrise-tools/go-steputils/stepconf"
)

func main() {
	var config Config
	if err := stepconf.Parse(&config); err != nil {
		log.Errorf("Configuration error: %s\n", err)
		os.Exit(7)
	}
	config.DeployDir = os.Getenv("BITRISE_DEPLOY_DIR")
	stepconf.Print(config)

	if err := ensureAndroidSdkSetup(); err != nil {
		log.Errorf("Could not setup Android SDK, error: %s", err)
		os.Exit(6)
	}

	if err := ensureMacOSSetup(); err != nil {
		log.Errorf("Could not setup macOS environment, error: %s", err)
		os.Exit(6)
	}

	flutterSdkDir, err := getSdkDestinationDir()
	if err != nil {
		log.Errorf("Could not Flutter SDK destination directory, error: %s", err)
		os.Exit(5)
	}

	flutterSdkExists, err := pathutil.IsDirExists(flutterSdkDir)
	if err != nil {
		log.Errorf("Could not check if Flutter SDK is installed, error: %s", err)
		os.Exit(1)
	}

	if !flutterSdkExists {
		if err := extractSdk(config.Version, flutterSdkDir); err != nil {
			log.Errorf("Could not extract Flutter SDK, error: %s", err)
			os.Exit(2)
		}
	} else {
		log.Infof("Flutter SDK directory already exists, skipping installation.")
	}

	buildStarted := time.Now()

	for _, flutterCommand := range config.Commands {
		log.Infof("Executing Flutter command: %s", flutterCommand)

		flutterExecutablePath := filepath.Join(flutterSdkDir, "bin/flutter")
		bashCommand := fmt.Sprintf("%s %s", flutterExecutablePath, flutterCommand)
		err := command.RunCommandInDir(config.WorkingDir, "bash", "-c", bashCommand)
		if err != nil {
			log.Errorf("Flutter invocation failed, error: %s", err)
			os.Exit(3)
		}
	}

	moveApkFiles(config, buildStarted)
}

func moveApkFiles(configs Config, gradleStarted time.Time) {
	// Move apk files
	fmt.Println()
	log.Infof("Move apk files...")
	apkFiles, err := find(".", configs.ApkFileIncludeFilter, configs.ApkFileExcludeFilter)
	if err != nil {
		failf("Failed to find apk files, error: %s", err)
	}

	if len(apkFiles) == 0 {
		log.Warnf("No file name matched apk filters")
	}

	lastCopiedApkFile := ""
	copiedApkFiles := []string{}
	for _, apkFile := range apkFiles {
		fi, err := os.Lstat(apkFile)
		if err != nil {
			failf("Failed to get file info, error: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			log.Warnf("skipping: %s, modified before the gradle task has started", apkFile)
			continue
		}

		ext := filepath.Ext(apkFile)
		baseName := filepath.Base(apkFile)
		baseName = strings.TrimSuffix(baseName, ext)

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			failf("Failed to create apk deploy path, error: %s", err)
		}

		log.Printf("copy %s to %s", apkFile, deployPth)
		if err := command.CopyFile(apkFile, deployPth); err != nil {
			failf("Failed to copy apk, error: %s", err)
		}

		lastCopiedApkFile = deployPth
		copiedApkFiles = append(copiedApkFiles, deployPth)
	}

	if lastCopiedApkFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_APK_PATH", lastCopiedApkFile); err != nil {
			failf("Failed to export enviroment (BITRISE_APK_PATH), error: %s", err)
		}
		log.Donef("The apk path is now available in the Environment Variable: $BITRISE_APK_PATH (value: %s)", lastCopiedApkFile)
	}
	if len(copiedApkFiles) > 0 {
		apkList := strings.Join(copiedApkFiles, "|")
		if err := exportEnvironmentWithEnvman("BITRISE_APK_PATH_LIST", apkList); err != nil {
			failf("Failed to export enviroment (BITRISE_APK_PATH_LIST), error: %s", err)
		}
		log.Donef("The apk paths list is now available in the Environment Variable: $BITRISE_APK_PATH_LIST (value: %s)", apkList)
	}

	testApkFiles, err := find(".", configs.TestApkFileIncludeFilter, configs.TestApkFileExcludeFilter)
	if err != nil {
		failf("Failed to find test apk files, error: %s", err)
	}

	if len(testApkFiles) == 0 {
		log.Warnf("No file name matched test apk filters")
	}

	lastCopiedTestApkFile := ""
	for _, apkFile := range testApkFiles {
		fi, err := os.Lstat(apkFile)
		if err != nil {
			failf("Failed to get file info, error: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			log.Warnf("skipping: %s, modified before the gradle task has started", apkFile)
			continue
		}

		ext := filepath.Ext(apkFile)
		baseName := filepath.Base(apkFile)
		baseName = strings.TrimSuffix(baseName, ext)

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			failf("Failed to create apk deploy path, error: %s", err)
		}

		log.Printf("copy %s to %s", apkFile, deployPth)
		if err := command.CopyFile(apkFile, deployPth); err != nil {
			failf("Failed to copy apk, error: %s", err)
		}

		lastCopiedTestApkFile = deployPth
	}
	if lastCopiedTestApkFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_TEST_APK_PATH", lastCopiedTestApkFile); err != nil {
			failf("Failed to export enviroment (BITRISE_TEST_APK_PATH), error: %s", err)
		}
		log.Donef("The apk path is now available in the Environment Variable: $BITRISE_TEST_APK_PATH (value: %s)", lastCopiedTestApkFile)
	}

	// Move mapping files
	log.Infof("Move mapping files...")
	mappingFiles, err := find(".", configs.MappingFileIncludeFilter, configs.MappingFileExcludeFilter)
	if err != nil {
		failf("Failed to find mapping files, error: %s", err)
	}

	if len(mappingFiles) == 0 {
		log.Printf("No mapping file matched the filters")
	}

	lastCopiedMappingFile := ""
	for _, mappingFile := range mappingFiles {
		fi, err := os.Lstat(mappingFile)
		if err != nil {
			failf("Failed to get file info, error: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			log.Warnf("skipping: %s, modified before the gradle task has started", mappingFile)
			continue
		}

		ext := filepath.Ext(mappingFile)
		baseName := filepath.Base(mappingFile)
		baseName = strings.TrimSuffix(baseName, ext)

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			failf("Failed to create mapping deploy path, error: %s", err)
		}

		log.Printf("copy %s to %s", mappingFile, deployPth)
		if err := command.CopyFile(mappingFile, deployPth); err != nil {
			failf("Failed to copy mapping file, error: %s", err)
		}

		lastCopiedMappingFile = deployPth
	}

	if lastCopiedMappingFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_MAPPING_PATH", lastCopiedMappingFile); err != nil {
			failf("Failed to export enviroment (BITRISE_MAPPING_PATH), error: %s", err)
		}
		log.Donef("The mapping path is now available in the Environment Variable: $BITRISE_MAPPING_PATH (value: %s)", lastCopiedMappingFile)
	}
}

func extractSdk(flutterVersion, flutterSdkDestinationDir string) error {
	log.Infof("Extracting Flutter SDK to %s", flutterSdkDestinationDir)

	versionComponents := strings.Split(flutterVersion, "-")
	channel := versionComponents[len(versionComponents)-1]

	flutterSdkSourceURL := fmt.Sprintf(
		"https://storage.googleapis.com/flutter_infra/releases/%s/%s/flutter_%s_v%s.%s",
		channel,
		getFlutterPlatform(),
		getFlutterPlatform(),
		flutterVersion,
		getArchiveExtension())

	flutterSdkParentDir := filepath.Join(flutterSdkDestinationDir, "..")

	if runtime.GOOS == "darwin" {
		return command.DownloadAndUnZIP(flutterSdkSourceURL, flutterSdkParentDir)
	} else if runtime.GOOS == "linux" {
		return downloadAndUnTarXZ(flutterSdkSourceURL, flutterSdkParentDir)
	} else {
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func find(dir, nameInclude, nameExclude string) ([]string, error) {
	cmdSlice := []string{"find", dir}
	cmdSlice = append(cmdSlice, "-path", nameInclude)

	for _, exclude := range strings.Split(nameExclude, "\n") {
		if exclude != "" {
			cmdSlice = append(cmdSlice, "!", "-path", exclude)
		}
	}

	log.Printf(command.PrintableCommandArgs(false, cmdSlice))

	out, err := command.New(cmdSlice[0], cmdSlice[1:]...).RunAndReturnTrimmedOutput()
	if err != nil {
		return []string{}, err
	}

	split := strings.Split(out, "\n")
	files := []string{}
	for _, item := range split {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			files = append(files, trimmed)
		}
	}

	return files, nil
}

func createDeployPth(deployDir, apkName string) (string, error) {
	deployPth := filepath.Join(deployDir, apkName)

	if exist, err := pathutil.IsPathExists(deployPth); err != nil {
		return "", err
	} else if exist {
		return "", fmt.Errorf("file already exists at: %s", deployPth)
	}

	return deployPth, nil
}

func findDeployPth(deployDir, baseName, ext string) (string, error) {
	deployPth := ""
	retryApkName := baseName + ext

	err := retry.Times(10).Wait(1 * time.Second).Try(func(attempt uint) error {
		if attempt > 0 {
			log.Warnf("  Retrying...")
		}

		pth, pathErr := createDeployPth(deployDir, retryApkName)
		if pathErr != nil {
			log.Warnf("  %d attempt failed:", attempt+1)
			log.Printf(pathErr.Error())
		}

		t := time.Now()
		retryApkName = baseName + t.Format("20060102150405") + ext
		deployPth = pth

		return pathErr
	})

	return deployPth, err
}

func exportEnvironmentWithEnvman(keyStr, valueStr string) error {
	cmd := command.New("envman", "add", "--key", keyStr)
	cmd.SetStdin(strings.NewReader(valueStr))
	return cmd.Run()
}

func failf(message string, args ...interface{}) {
	log.Errorf(message, args...)
	os.Exit(1)
}
