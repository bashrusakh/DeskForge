#!/bin/bash
# Build Agent: Linux (.rpm) + Android (.apk)
# Polls /rdgen-data/jobs/ for new build jobs, processes them, outputs to /rdgen-data/output/
# Note: do NOT use `set -e` — cargo's exit code or any subcommand failure would kill PID 1
# and restart the whole container. We use explicit error handling instead.

JOBS_DIR="/rdgen-data/jobs"
OUTPUT_DIR="/rdgen-data/output"
CACHE_DIR="/rustdesk-cache"
PATCHES_DIR="/rdgen-data/patches"

mkdir -p "$JOBS_DIR" "$OUTPUT_DIR" "$CACHE_DIR"

echo "Build Agent Linux: started. Watching $JOBS_DIR for jobs..."

process_job() {
    local job_file="$1"
    local job_id
    job_id=$(basename "$job_file" .json)
    local platform version host key api_server relay_server app_name custom_json
    platform=$(jq -r '.platform' "$job_file")
    version=$(jq -r '.version' "$job_file")
    host=$(jq -r '.host' "$job_file")
    key=$(jq -r '.key' "$job_file")
    api_server=$(jq -r '.api_server' "$job_file")
    relay_server=$(jq -r '.relay_server' "$job_file")
    app_name=$(jq -r '.app_name' "$job_file")
    custom_json=$(jq -r '.custom_json' "$job_file")

    local output_dir="$OUTPUT_DIR/$job_id"
    mkdir -p "$output_dir"

    echo "Job $job_id: platform=$platform version=$version app=$app_name"

    if [ -z "$platform" ] || [ "$platform" = "null" ]; then
        echo "Job $job_id: invalid job file (no platform), skipping"
        echo "failed" > "$output_dir/status"
        rm -f "$job_file"
        return
    fi

    if [ "$platform" != "linux" ] && [ "$platform" != "android" ]; then
        echo "Job $job_id: skipping $platform job on Linux agent"
        rm -f "$job_file"
        return
    fi

    # Update status to building
    echo "building" > "$output_dir/status"

    # Use pre-baked rustdesk source (baked into image during Docker build)
    local src_dir="/opt/rustdesk-src"
    echo "Using baked rustdesk source at $src_dir (version: baked)"

    cd "$src_dir" || { echo "Job $job_id: cd to $src_dir failed"; echo "failed" > "$output_dir/status"; rm -f "$job_file"; return; }

    # Apply patches (optional)
    if [ -d "$PATCHES_DIR" ]; then
        for patch in "$PATCHES_DIR"/*.diff; do
            if [ -f "$patch" ]; then
                echo "Applying patch: $(basename "$patch")"
                patch -p1 < "$patch" >/dev/null 2>&1 || echo "Warning: patch $(basename "$patch") failed (continuing)"
            fi
        done
    fi

    # Write custom config
    mkdir -p flutter/lib
    echo "$custom_json" > flutter/lib/custom.json

    case "$platform" in
        linux)
            echo "Building Linux binary..."
            if cargo build --release --target x86_64-unknown-linux-gnu --features linux-pkg-config; then
                if cp target/x86_64-unknown-linux-gnu/release/rustdesk "$output_dir/rustdesk"; then
                    echo "done" > "$output_dir/status"
                    rm -f "$job_file"
                    echo "Job $job_id finished successfully"
                else
                    echo "Job $job_id: cp failed"
                    echo "failed" > "$output_dir/status"
                    rm -f "$job_file"
                fi
            else
                echo "Job $job_id: cargo build failed"
                echo "failed" > "$output_dir/status"
                rm -f "$job_file"
            fi
            # TODO: .rpm/.deb packaging
            ;;
        android)
            echo "Building Android .apk..."
            export ANDROID_HOME=/opt/android-sdk
            export ANDROID_NDK_HOME=/opt/android-sdk/ndk/26.1.10909125
            if flutter build apk --release; then
                if cp build/app/outputs/flutter-apk/app-release.apk "$output_dir/rustdesk.apk"; then
                    echo "done" > "$output_dir/status"
                    rm -f "$job_file"
                    echo "Job $job_id finished successfully"
                else
                    echo "Job $job_id: cp failed"
                    echo "failed" > "$output_dir/status"
                    rm -f "$job_file"
                fi
            else
                echo "Job $job_id: flutter build failed"
                echo "failed" > "$output_dir/status"
                rm -f "$job_file"
            fi
            ;;
    esac
}

# Use glob that returns empty if no match (so for loop doesn't iterate over literal pattern)
shopt -s nullglob

while true; do
    for job_file in "$JOBS_DIR"/*.json; do
        if [ -f "$job_file" ]; then
            process_job "$job_file"
        fi
    done
    sleep 5
done
