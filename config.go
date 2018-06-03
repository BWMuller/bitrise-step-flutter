package main

// Config ...
type Config struct {
	Version    string   `env:"version,required"`
	WorkingDir string   `env:"working_dir,dir"`
	Commands   []string `env:"commands,required"`

	ApkFileIncludeFilter     string `env:"apk_file_include_filter"`
	ApkFileExcludeFilter     string `env:"apk_file_exclude_filter"`
	TestApkFileIncludeFilter string `env:"test_apk_file_include_filter"`
	TestApkFileExcludeFilter string `env:"test_apk_file_exclude_filter"`
	MappingFileIncludeFilter string `env:"mapping_file_include_filter"`
	MappingFileExcludeFilter string `env:"mapping_file_exclude_filter"`

	// Other configs
	DeployDir string
}

func (config *Config) stripEmptyCommands() {
	var strippedCommands []string
	for _, command := range config.Commands {
		if command != "" {
			strippedCommands = append(strippedCommands, command)
		}
	}
	config.Commands = strippedCommands
}
