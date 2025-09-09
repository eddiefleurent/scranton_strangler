#!/bin/bash
set -e

echo "🚀 Deploying Scranton Strangler Binary to Unraid..."
echo "=================================================="

# Configuration
UNRAID_HOST="unraid"
UNRAID_APP_PATH="/mnt/user/appdata/scranton-strangler"
SERVICE_NAME="scranton-strangler"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warn() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
    exit 1
}

# Check prerequisites
echo "📋 Checking prerequisites..."

if ! ssh "$UNRAID_HOST" "echo 'SSH connection successful'" &> /dev/null; then
    log_error "Cannot SSH to $UNRAID_HOST. Check your SSH key setup."
fi

if [ ! -f "config.yaml" ]; then
    log_error "config.yaml not found. Copy from config.yaml.example and configure your Tradier credentials."
fi

log_info "Prerequisites check passed"

# Build the Go binary for Linux
echo "🔨 Building Go binary for Linux..."
make build-prod

if [ ! -f "scranton-strangler" ]; then
    log_error "Build failed - binary not found"
fi

log_info "Build completed successfully"

# Create app directory on Unraid
echo "📁 Creating app directory on Unraid..."
ssh "$UNRAID_HOST" "mkdir -p $UNRAID_APP_PATH/{data,logs}"

# Stop existing service if running
echo "🛑 Stopping existing service..."
ssh "$UNRAID_HOST" "if [ -f $UNRAID_APP_PATH/stop-service.sh ]; then $UNRAID_APP_PATH/stop-service.sh; else killall scranton-strangler 2>/dev/null || echo 'Service not running'; fi"

# Copy binary and config to Unraid
echo "📦 Copying files to Unraid..."
scp scranton-strangler "$UNRAID_HOST:$UNRAID_APP_PATH/"
scp config.yaml "$UNRAID_HOST:$UNRAID_APP_PATH/"

# Make binary executable
ssh "$UNRAID_HOST" "chmod +x $UNRAID_APP_PATH/scranton-strangler"

# Create systemd-style service script for Unraid
echo "⚙️  Creating service script..."
ssh "$UNRAID_HOST" "cat > $UNRAID_APP_PATH/start-service.sh << 'EOF'
#!/bin/bash
cd $UNRAID_APP_PATH
export TZ=America/New_York
export CONFIG_PATH=$UNRAID_APP_PATH/config.yaml
export LOG_LEVEL=info

# Create data directory if it doesn't exist
mkdir -p data logs

# Initialize positions file if it doesn't exist
if [ ! -f data/positions.json ]; then
    echo '{\"positions\": []}' > data/positions.json
fi

# Start the bot
exec ./scranton-strangler > logs/bot.log 2>&1 &
echo \$! > scranton-strangler.pid

echo \"Scranton Strangler started with PID \$(cat scranton-strangler.pid)\"
EOF"

ssh "$UNRAID_HOST" "chmod +x $UNRAID_APP_PATH/start-service.sh"

# Create stop script
ssh "$UNRAID_HOST" "cat > $UNRAID_APP_PATH/stop-service.sh << 'EOF'
#!/bin/bash
cd $UNRAID_APP_PATH
if [ -f scranton-strangler.pid ]; then
    PID=\$(cat scranton-strangler.pid)
    if kill \$PID 2>/dev/null; then
        echo \"Stopped Scranton Strangler (PID \$PID)\"
    else
        echo \"Process \$PID not found\"
    fi
    rm -f scranton-strangler.pid
else
    echo \"PID file not found\"
fi
EOF"

ssh "$UNRAID_HOST" "chmod +x $UNRAID_APP_PATH/stop-service.sh"

# Add to Unraid's go script for auto-start
echo "🔄 Adding to Unraid auto-start..."
ssh "$UNRAID_HOST" "grep -q 'scranton-strangler' /boot/config/go 2>/dev/null || echo '$UNRAID_APP_PATH/start-service.sh' >> /boot/config/go"

# Start the service
echo "🎬 Starting service..."
ssh "$UNRAID_HOST" "$UNRAID_APP_PATH/start-service.sh"

# Wait for service to start
echo "⏳ Waiting for service to start..."
sleep 3

# Check if service is running
if ssh "$UNRAID_HOST" "pgrep -f scranton-strangler" >/dev/null; then
    log_info "Service started successfully!"
    
    # Show service status
    echo "📊 Service Status:"
    ssh "$UNRAID_HOST" "ps aux | grep scranton-strangler | grep -v grep"
    
    # Show recent logs
    echo ""
    echo "📝 Recent logs:"
    ssh "$UNRAID_HOST" "tail -20 $UNRAID_APP_PATH/logs/bot.log 2>/dev/null || echo 'No logs yet'"
else
    log_error "Service failed to start"
fi

echo ""
log_info "Binary deployment complete!"
echo "🔗 Monitor logs: ssh $UNRAID_HOST 'tail -f $UNRAID_APP_PATH/logs/bot.log'"
echo "🔗 Stop service: ssh $UNRAID_HOST '$UNRAID_APP_PATH/stop-service.sh'"
echo "🔗 Check positions: ssh $UNRAID_HOST 'cat $UNRAID_APP_PATH/data/positions.json'"

# Cleanup
rm -f scranton-strangler

log_info "All done! 🎉"