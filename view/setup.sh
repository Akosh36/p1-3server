#!/bin/bash
# Quick setup for monitoring dashboard

echo "================================================"
echo "Monitoring Dashboard Setup"
echo "================================================"
echo ""

# Check if Python is installed
if ! command -v python3 &> /dev/null; then
    echo "❌ Python 3 is not installed"
    echo "   Please install Python 3.7 or higher"
    exit 1
fi

echo "✓ Python 3 found"
python3 --version

# Check if pip is installed
if ! command -v pip3 &> /dev/null; then
    echo "❌ pip3 is not installed"
    echo "   Please install pip"
    exit 1
fi

echo "✓ pip3 found"
echo ""

# Create virtual environment (optional but recommended)
echo "Setting up Python environment..."
python3 -m venv venv

# Activate virtual environment
echo "Activating virtual environment..."
source venv/bin/activate

# Install requirements
echo "Installing dependencies..."
pip install -r requirements.txt

echo ""
echo "================================================"
echo "✓ Setup Complete!"
echo "================================================"
echo ""
echo "To start the monitoring dashboard:"
echo ""
echo "  1. Run: ./run.sh"
echo "  2. Open browser: http://localhost:5000"
echo ""
echo "Make sure the web servers are running:"
echo "  ../control-scripts/start.sh"
echo ""
