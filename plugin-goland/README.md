# Samurai GoLand Plugin

A GoLand plugin that adds gutter icons for running Samurai BDD tests directly from the editor.

## Features

- **Gutter icons** next to `s.Test()` and `s.Then()` calls in test files
- **One-click execution** of specific test paths
- **Debug support** for stepping through individual test paths
- **Test result navigation** — click a subtest in results to jump to source
- **Pass/fail status** — gutter icons update to show green checkmark or red X after running
- Uses GoLand's native test runner with `-run` flag

## Installation

### From Source

1. Clone the repository
2. Build the plugin:
   ```bash
   cd plugin-goland
   ./gradlew buildPlugin
   ```
3. Install the built plugin from `build/distributions/samurai-goland-*.zip`:
   - In GoLand: Settings > Plugins > gear icon > Install Plugin from Disk...

### Quick Install

```bash
cd plugin-goland
./install-plugin.sh
```

## Usage

1. Open a Go test file that uses samurai
2. Look for green play icons in the gutter next to `s.Test()` and `s.Then()` calls
3. Click an icon to see options:
   - **Run** — Execute this specific test path with clickable results
   - **Debug** — Debug this specific test path with GoLand's debugger
4. After running, gutter icons update to show pass/fail status

### Example

```go
func TestDatabase(t *testing.T) {
    samurai.Run(t, func(s *samurai.S) {
        var db *DB

        s.When(func(w samurai.W) {          // no icon (setup)
            db = setupDB()
            w.Cleanup(func() { db.Close() })
        })

        s.Test("create user", func(s *samurai.S) {  // play icon
            var user *User

            s.When(func(w samurai.W) {
                user = db.CreateUser("test@example.com")
            })

            s.Then("has email", func(c samurai.C) {  // play icon
                assert.Equal(c.T(), "test@example.com", user.Email)
            })
        })

        s.Then("can query all", func(c samurai.C) {  // play icon
            _, err := db.QueryAll()
            assert.NoError(c.T(), err)
        })
    })
}
```

## How It Works

The plugin:
1. Scans Go test files for `s.Test("name", ...)` and `s.Then("name", ...)` method calls
2. Walks up the PSI closure chain to build the full path (e.g., `TestDatabase/create_user/has_email`)
3. Creates a run configuration with `-run "^TestDatabase/create_user/has_email$"`
4. Executes via GoLand's Go test runner
5. Tracks test results to show pass/fail icons in the gutter

## Requirements

- GoLand 2025.3 or later
- Go plugin (bundled with GoLand)

## Development

### Building

```bash
./gradlew buildPlugin
```

### Running in Sandbox

```bash
./gradlew runIde
```

This launches a sandboxed GoLand instance with the plugin installed.

## Project Structure

```
plugin-goland/
├── build.gradle.kts
├── settings.gradle.kts
├── gradle.properties
├── install-plugin.sh
├── src/main/
│   ├── kotlin/com/samurai/plugin/
│   │   ├── SamuraiRunLineMarkerProvider.kt   # Gutter icon provider
│   │   ├── SamuraiPathResolver.kt            # Path extraction logic
│   │   ├── SamuraiTestLocator.kt             # Test result navigation
│   │   ├── SamuraiTestResultCache.kt         # Pass/fail status cache
│   │   ├── SamuraiRunConfiguration.kt        # Run configuration
│   │   ├── SamuraiConfigurationType.kt       # Configuration type
│   │   ├── SamuraiRunningState.kt            # Test execution
│   │   └── SamuraiRunConfigurationProducer.kt # Intercepts native run actions
│   └── resources/
│       └── META-INF/plugin.xml
└── README.md
```

## License

MIT License - see the main samurai repository for details.
