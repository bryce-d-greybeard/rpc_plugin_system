package main

import "strings"

var flagsWithValues = map[string]struct{}{
	"-runtime-dir": {},
	"-plugin-id":   {},
	"-message":     {},
	"-level":       {},
	"-component":   {},
	"-event":       {},
	"-method":      {},
	"-limit":       {},
	"-format":      {},
	"-since":       {},
}

var boolFlags = map[string]struct{}{
	"-reverse": {},
	"-summary": {},
}

func splitCommandArgs(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 >= len(args) {
				return "", args[:i]
			}
			return args[i+1], append(args[:i], args[i+2:]...)
		}
		if takesValue(arg) {
			i++
			continue
		}
		if isBoolFlag(arg) {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg, append(args[:i], args[i+1:]...)
	}
	return "", args
}

func takesValue(arg string) bool {
	if _, ok := flagsWithValues[arg]; ok {
		return true
	}
	for flagName := range flagsWithValues {
		if strings.HasPrefix(arg, flagName+"=") {
			return false
		}
	}
	return false
}

func isBoolFlag(arg string) bool {
	if _, ok := boolFlags[arg]; ok {
		return true
	}
	for flagName := range boolFlags {
		if arg == flagName+"=true" || arg == flagName+"=false" {
			return true
		}
	}
	return false
}
