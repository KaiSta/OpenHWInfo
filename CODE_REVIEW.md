# OpenHWInfo Code Review

## Overview
OpenHWInfo is a Go-based web application that provides hardware monitoring information for CPU and GPU components. It serves a dashboard at port 8080 displaying real-time system metrics.

## Issues Found and Recommendations

### 1. **Error Handling Improvements**

#### In `cpuinfo/cpu.go`:
- **Line 26-27**: Using `log.Fatal(err)` when the `sensors` command fails is too aggressive. This will crash the entire application if sensors data cannot be retrieved.
  - **Recommendation**: Replace with proper error handling that logs the error but allows the application to continue with partial data.

#### In `amd/amd.go`:
- **Line 27-30**: Error from `lspci` command is logged but execution continues. The function returns a default value, which is good.
- **Line 94-106**: Code duplicates temperature parsing logic (lines 71-81 and 94-106). The second block doesn't check if hwmonPath exists first.
  - **Recommendation**: Remove the duplicate code block. The device path operations should only execute if hwmonPath is found.

### 2. **Code Duplication**

#### In `amd/amd.go`:
- **Lines 71-81 and 94-106**: Temperature and frequency parsing is duplicated.
  - **Recommendation**: Remove the duplicate code block (lines 94-106).

### 3. **Debugging Code Left in Production**

#### In `amd/amd.go`:
- **Lines 93, 95, 97**: Debug print statements that should be removed or wrapped in debug flags.
  - **Recommendation**: Remove these debugging lines or use a logging level to control when they appear.

### 4. **Potential Bugs**

#### In `amd/amd.go`:
- **Line 61**: `probeAMD()` is called but the result is not properly checked before using `cardIndex`.
- **Lines 93-97**: Code assumes hwmonPath exists when accessing `/device` subdirectory, but this wasn't verified in the duplicate code block.
  - **Recommendation**: Add proper error checking and only execute device path operations if hwmonPath is valid.

#### In `cpuinfo/cpu.go`:
- **Line 61**: Accesses `lines[i+2]` without bounds checking. If i is near the end of the array, this will panic.
  - **Recommendation**: Add bounds checking before accessing lines[i+2].

### 5. **Type Safety Issues**

#### In `nvidia/nvidia.go`:
- **Line 63-74**: Directly assigns fields from CSV without type conversion or validation.
  - **Recommendation**: Add type conversion and error handling for each field.

### 6. **Security Considerations**

#### In `amd/amd.go`:
- **Lines 25, 45**: Uses shell commands (`lspci`, `glxinfo`) which could be vulnerable to command injection if user input were involved.
  - **Recommendation**: While not currently an issue (no user input), consider using direct exec.Command without shell for these commands.

### 7. **Performance Issues**

#### In `website/index.html`:
- **Lines 64-66**: Auto-refreshes every 5 seconds, causing unnecessary server load and bandwidth usage.
  - **Recommendation**: Consider removing or making this configurable (e.g., via URL parameter).

### 8. **Code Quality Issues**

#### In all files:
- Missing package-level documentation comments explaining the purpose of each package.
- Missing function-level documentation for exported functions.
- No tests are present for any of the modules.

#### Recommendations:
1. Add Go doc comments for all exported functions and types
2. Create unit tests for parsing logic
3. Consider using structs with proper tags instead of map[string]string for data passing

### 9. **Maintainability Issues**

#### In `main.go`:
- Hardcoded port (8080) should be configurable.
- No graceful shutdown handling.
- Template files are loaded from disk on every request.

#### Recommendations:
1. Make port configurable via environment variable or flag
2. Add proper signal handling for graceful shutdown
3. Cache compiled templates instead of parsing on each request

### 10. **Inconsistent Error Handling**

Different modules handle errors differently:
- `cpuinfo`: Uses log.Fatal (crashes)
- `amd`: Logs and continues with defaults
- `nvidia`: Logs and returns empty

#### Recommendation: Standardize error handling approach across all modules.

## Summary of Critical Issues

1. **Crash on sensors failure** (`cpuinfo/cpu.go` line 27) - High priority
2. **Duplicate code in AMD module** - Should be removed
3. **Debug statements left in production** - Should be cleaned up
4. **Array bounds issue** (`cpuinfo/cpu.go` line 61) - Potential crash
5. **Unchecked hwmonPath usage** (`amd/amd.go` lines 93-106) - Potential crash

## Recommendations for Next Steps

1. Fix critical error handling issues first
2. Remove duplicate code and debug statements
3. Add proper bounds checking
4. Standardize error handling across modules
5. Add unit tests
6. Improve documentation