package server

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/amitschendel/curing/pkg/common"
)

// CommandConfig represents the server's command configuration
type CommandConfig struct {
	DefaultCommands []common.Command                    `json:"default_commands"`
	GroupCommands   map[string][]common.Command         `json:"group_commands"`
	ClientSpecific  map[string][]common.Command         `json:"client_specific"`
}

// CommandConfigRaw represents the raw JSON structure for command configuration
type CommandConfigRaw struct {
	DefaultCommands []CommandDefinition                 `json:"default_commands"`
	GroupCommands   map[string][]CommandDefinition      `json:"group_commands"`
	ClientSpecific  map[string][]CommandDefinition      `json:"client_specific"`
}

// CommandDefinition represents a command in the JSON configuration
type CommandDefinition struct {
	Type    string            `json:"type"`
	ID      string            `json:"id"`
	Path    string            `json:"path,omitempty"`
	Command string            `json:"command,omitempty"`
	Content string            `json:"content,omitempty"`
	OldPath string            `json:"oldpath,omitempty"`
	NewPath string            `json:"newpath,omitempty"`
}

// LoadCommandConfig loads the command configuration from a JSON file
func LoadCommandConfig(filePath string) (*CommandConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not open command config file: %v", err)
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("could not read command config file: %v", err)
	}

	var rawConfig CommandConfigRaw
	if err := json.Unmarshal(bytes, &rawConfig); err != nil {
		return nil, fmt.Errorf("could not unmarshal command config JSON: %v", err)
	}

	// Convert raw config to actual command objects
	config := &CommandConfig{
		DefaultCommands: make([]common.Command, 0),
		GroupCommands:   make(map[string][]common.Command),
		ClientSpecific:  make(map[string][]common.Command),
	}

	// Convert default commands
	for _, cmdDef := range rawConfig.DefaultCommands {
		cmd, err := convertCommandDefinition(cmdDef)
		if err != nil {
			return nil, fmt.Errorf("error converting default command %s: %v", cmdDef.ID, err)
		}
		config.DefaultCommands = append(config.DefaultCommands, cmd)
	}

	// Convert group commands
	for groupName, cmdDefs := range rawConfig.GroupCommands {
		config.GroupCommands[groupName] = make([]common.Command, 0)
		for _, cmdDef := range cmdDefs {
			cmd, err := convertCommandDefinition(cmdDef)
			if err != nil {
				return nil, fmt.Errorf("error converting group command %s in group %s: %v", cmdDef.ID, groupName, err)
			}
			config.GroupCommands[groupName] = append(config.GroupCommands[groupName], cmd)
		}
	}

	// Convert client-specific commands
	for clientID, cmdDefs := range rawConfig.ClientSpecific {
		config.ClientSpecific[clientID] = make([]common.Command, 0)
		for _, cmdDef := range cmdDefs {
			cmd, err := convertCommandDefinition(cmdDef)
			if err != nil {
				return nil, fmt.Errorf("error converting client-specific command %s for client %s: %v", cmdDef.ID, clientID, err)
			}
			config.ClientSpecific[clientID] = append(config.ClientSpecific[clientID], cmd)
		}
	}

	return config, nil
}

// convertCommandDefinition converts a CommandDefinition to a common.Command
func convertCommandDefinition(cmdDef CommandDefinition) (common.Command, error) {
	switch cmdDef.Type {
	case "readfile":
		return common.ReadFile{
			Id:   cmdDef.ID,
			Path: cmdDef.Path,
		}, nil
	case "writefile":
		return common.WriteFile{
			Id:      cmdDef.ID,
			Path:    cmdDef.Path,
			Content: cmdDef.Content,
		}, nil
	case "execute":
		return common.Execute{
			Id:      cmdDef.ID,
			Command: cmdDef.Command,
		}, nil
	case "symlink":
		return common.Symlink{
			Id:      cmdDef.ID,
			OldPath: cmdDef.OldPath,
			NewPath: cmdDef.NewPath,
		}, nil
	default:
		return nil, fmt.Errorf("unknown command type: %s", cmdDef.Type)
	}
}

// GetCommandsForClient returns the commands that should be sent to a specific client
func (c *CommandConfig) GetCommandsForClient(agentID string, groups []string) []common.Command {
	var commands []common.Command

	// 1. Client-specific commands (highest priority)
	if clientCmds, exists := c.ClientSpecific[agentID]; exists {
		commands = append(commands, clientCmds...)
	}

	// 2. Group commands
	for _, group := range groups {
		if groupCmds, exists := c.GroupCommands[group]; exists {
			commands = append(commands, groupCmds...)
		}
	}

	// 3. Default commands (if no specific commands found)
	if len(commands) == 0 {
		commands = c.DefaultCommands
	}

	return commands
}
