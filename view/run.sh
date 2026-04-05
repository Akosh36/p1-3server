#!/bin/bash
# Run Python monitoring dashboard

echo "📊 Starting Monitoring Dashboard..."
echo ""

# Get the directory where this script is located
SCRIPT_DIR="$(dirname "$0")"
cd "$SCRIPT_DIR"

# Activate virtual environment
if [ -d "venv" ]; then
    echo "Activating virtual environment..."
    source venv/bin/activate
else
    echo "❌ Virtual environment not found. Creating it..."
    python3 -m venv venv
    source venv/bin/activate
    echo "Installing dependencies..."
    pip install -r requirements.txt
fi

echo "================================================"
echo "Flask Monitoring Dashboard"
echo "================================================"
echo ""
echo "🔗 Access the dashboard at:"
echo "   http://localhost:5000"
echo ""
echo "📊 Features:"
echo "   ✓ Real-time container status"
echo "   ✓ Server health monitoring"
echo "   ✓ Performance metrics & charts"
echo "   ✓ Response time tracking"
echo "   ✓ System resource usage"
echo "   ✓ Server management & control"
echo "   ✓ File upload capabilities"
echo "   ✓ Load balancer configuration"
echo "   ✓ Host monitoring tools"
echo ""
echo "💡 Press Ctrl+C to stop"
echo "================================================"
echo ""

python3 monitor.py
