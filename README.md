# CBZOptimizer

CBZOptimizer is a Go-based tool designed to optimize CBZ (Comic Book Zip) and CBR (Comic Book RAR) files by converting images to a specified format and quality. This tool is useful for reducing the size of comic book archives while maintaining acceptable image quality.

**Note**: CBR files are supported as input but are always converted to CBZ format for output.

## Features

- Convert images within CBZ and CBR files to different formats (e.g., WebP).
- Support for multiple archive formats including CBZ and CBR (CBR files are converted to CBZ format).
- Adjust the quality of the converted images.
- Process multiple chapters in parallel.
- Option to override the original files (CBR files are converted to CBZ and original CBR is deleted).
- Watch a folder for new CBZ/CBR files and optimize them automatically.
- Set time limits for chapter conversion to avoid hanging on problematic files.

## Installation

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/belphemur/CBZOptimizer/releases).

### Docker

Pull the Docker image:

```sh
docker pull ghcr.io/belphemur/cbzoptimizer:latest
```

## Usage

### Command Line Interface

The tool provides CLI commands to optimize and watch CBZ/CBR files. Below are examples of how to use them:

#### Optimize Command

Optimize all CBZ/CBR files in a folder recursively:

```sh
cbzconverter optimize [folder] --quality 85 --parallelism 2 --override --format webp --split
```

The format flag can be specified in multiple ways:

```sh
# Using space-separated syntax
cbzconverter optimize [folder] --format webp

# Using short form with space
cbzconverter optimize [folder] -f webp

# Using equals syntax
cbzconverter optimize [folder] --format=webp

# Format is case-insensitive
cbzconverter optimize [folder] --format WEBP
```

With timeout to avoid hanging on problematic chapters:

```sh
cbzconverter optimize [folder] --timeout 10m --quality 85
```

Or with Docker:

```sh
docker run -v /path/to/comics:/comics ghcr.io/belphemur/cbzoptimizer:latest optimize /comics --quality 85 --parallelism 2 --override --format webp --split
```

#### Watch Command

Watch a folder for new CBZ/CBR files and optimize them automatically:

```sh
cbzconverter watch [folder] --quality 85 --override --format webp --split
```

Or with Docker:

```sh
docker run -v /path/to/comics:/comics ghcr.io/belphemur/cbzoptimizer:latest watch /comics --quality 85 --override --format webp --split
```

### Flags

- `--quality`, `-q`: Quality for conversion (0-100). Default is 85.
- `--parallelism`, `-n`: Number of chapters to convert in parallel. Default is 2.
- `--override`, `-o`: Override the original files. For CBZ files, overwrites the original. For CBR files, deletes the original CBR and creates a new CBZ. Default is false.
- `--split`, `-s`: Split long pages into smaller chunks. Default is false.
- `--format`, `-f`: Format to convert the images to (currently supports: webp). Default is webp.
  - Can be specified as: `--format webp`, `-f webp`, or `--format=webp`
  - Case-insensitive: `webp`, `WEBP`, and `WebP` are all valid
- `--timeout`, `-t`: Maximum time allowed for converting a single chapter (e.g., 30s, 5m, 1h). 0 means no timeout. Default is 0.
- `--log`, `-l`: Set log level; can be 'panic', 'fatal', 'error', 'warn', 'info', 'debug', or 'trace'. Default is info.

## Logging

CBZOptimizer uses structured logging with [zerolog](https://github.com/rs/zerolog) for consistent and performant logging output.

### Log Levels

You can control the verbosity of logging using either command-line flags or environment variables:

**Command Line:**

```sh
# Set log level to debug for detailed output
cbzconverter --log debug optimize [folder]

# Set log level to error for minimal output
cbzconverter --log error optimize [folder]
```

**Environment Variable:**

```sh
# Set log level via environment variable
LOG_LEVEL=debug cbzconverter optimize [folder]
```

**Docker:**

```sh
# Set log level via environment variable in Docker
docker run -e LOG_LEVEL=debug -v /path/to/comics:/comics ghcr.io/belphemur/cbzoptimizer:latest optimize /comics
```

### Available Log Levels

- `panic`: Logs panic level messages and above
- `fatal`: Logs fatal level messages and above
- `error`: Logs error level messages and above
- `warn`: Logs warning level messages and above
- `info`: Logs info level messages and above (default)
- `debug`: Logs debug level messages and above
- `trace`: Logs all messages including trace level

### Examples

```sh
# Default info level logging
cbzconverter optimize comics/

# Debug level for troubleshooting
cbzconverter --log debug optimize comics/

# Quiet operation (only errors and above)
cbzconverter --log error optimize comics/

# Using environment variable
LOG_LEVEL=warn cbzconverter optimize comics/

# Docker with debug logging
docker run -e LOG_LEVEL=debug -v /path/to/comics:/comics ghcr.io/belphemur/cbzoptimizer:latest optimize /comics
```

## Docker Image

The official Docker image is available at: `ghcr.io/belphemur/cbzoptimizer:latest`

### Docker Compose

You can use Docker Compose to run CBZOptimizer with persistent configuration. Create a `docker-compose.yml` file:

```yaml
version: '3.8'

services:
  cbzoptimizer:
    image: ghcr.io/belphemur/cbzoptimizer:latest
    container_name: cbzoptimizer
    environment:
      # Set log level (panic, fatal, error, warn, info, debug, trace)
      - LOG_LEVEL=info
      # User and Group ID for file permissions
      - PUID=99
      - PGID=100
    volumes:
      # Mount your comics directory
      - /path/to/your/comics:/comics
      # Optional: Mount a config directory for persistent settings
      - ./config:/config
    # Example: Optimize all comics in the /comics directory
    command: optimize /comics --quality 85 --parallelism 2 --override --format webp --split
    restart: unless-stopped
```

For watch mode, you can create a separate service:

```yaml
  cbzoptimizer-watch:
    image: ghcr.io/belphemur/cbzoptimizer:latest
    container_name: cbzoptimizer-watch
    environment:
      - LOG_LEVEL=info
      - PUID=99
      - PGID=100
    volumes:
      - /path/to/watch/directory:/watch
      - ./config:/config
    # Watch for new files and automatically optimize them
    command: watch /watch --quality 85 --override --format webp --split
    restart: unless-stopped
```

**Important Notes:**
- Replace `/path/to/your/comics` and `/path/to/watch/directory` with your actual directory paths
- The `PUID` and `PGID` environment variables control file permissions (default: 99/100)
- The `LOG_LEVEL` environment variable sets the logging verbosity
- For one-time optimization, remove the `restart: unless-stopped` line
- Watch mode only works on Linux systems

#### Running with Docker Compose

```sh
# Start the service (one-time optimization)
docker-compose up cbzoptimizer

# Start in detached mode
docker-compose up -d cbzoptimizer

# Start watch mode service
docker-compose up -d cbzoptimizer-watch

# View logs
docker-compose logs -f cbzoptimizer

# Stop services
docker-compose down
```

## Troubleshooting

If you encounter issues:

1. Use `--log debug` for detailed logging output
2. Check that all required dependencies are installed
3. Ensure proper file permissions for input/output directories
4. For Docker usage, verify volume mounts are correct

## Support

For issues and questions, please use [GitHub Issues](https://github.com/belphemur/CBZOptimizer/issues).

## License

This project is licensed under the MIT License. See the `LICENSE` file for details.
