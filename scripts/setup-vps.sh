#!/bin/bash
set -euo pipefail

# Set up a VPS for running Athanor CI with CloudHypervisor microVMs.
# Must be run as root on the target VPS.
# Usage: sudo ./scripts/setup-vps.sh

echo "=== Setting up Athanor CI VPS ==="

# 1. Install cloud-hypervisor
echo ">>> Installing cloud-hypervisor..."
if ! command -v cloud-hypervisor &> /dev/null; then
    CH_VERSION="v45.0"
    curl -sL "https://github.com/cloud-hypervisor/cloud-hypervisor/releases/download/${CH_VERSION}/cloud-hypervisor-static" \
        -o /usr/local/bin/cloud-hypervisor
    chmod +x /usr/local/bin/cloud-hypervisor
fi
cloud-hypervisor --version

# 2. Install virtiofsd
echo ">>> Installing virtiofsd..."
apt-get update -qq
apt-get install -y -qq virtiofsd

# 3. Download CH-compatible kernel
echo ">>> Downloading kernel..."
KERNEL_PATH="/var/lib/athanor/vmlinux"
if [ ! -f "$KERNEL_PATH" ]; then
    CH_VERSION="v45.0"
    curl -sL "https://github.com/cloud-hypervisor/cloud-hypervisor/releases/download/${CH_VERSION}/hypervisor-fw" \
        -o "$KERNEL_PATH"
fi

# 4. Install debootstrap for rootfs building
apt-get install -y -qq debootstrap

# 5. Set up network bridge
echo ">>> Setting up network bridge..."
ip link show br0 &>/dev/null || ip link add br0 type bridge
ip addr flush dev br0 2>/dev/null || true
ip addr add 192.168.100.1/24 dev br0 2>/dev/null || true
ip link set br0 up

# Enable IP forwarding
sysctl -w net.ipv4.ip_forward=1
echo "net.ipv4.ip_forward=1" > /etc/sysctl.d/99-athanor.conf

# NAT for VM internet access — detect the main interface
MAIN_IFACE=$(ip route show default | awk '/default/ {print $5}' | head -1)
iptables -t nat -C POSTROUTING -s 192.168.100.0/24 ! -o br0 -j MASQUERADE 2>/dev/null || \
    iptables -t nat -A POSTROUTING -s 192.168.100.0/24 ! -o br0 -j MASQUERADE

# Make iptables rules persistent
apt-get install -y -qq iptables-persistent
netfilter-persistent save

# 6. Create directories
mkdir -p /var/lib/athanor/vm-disks
mkdir -p /var/lib/athanor/workspaces
chown -R athanor:athanor /var/lib/athanor 2>/dev/null || true

# 7. Build rootfs (this takes a few minutes)
echo ">>> Building rootfs image..."
if [ ! -f /var/lib/athanor/rootfs.ext4 ]; then
    /usr/local/bin/athanor-build-rootfs || ./scripts/build-rootfs.sh
fi

# 8. Verify KVM access
echo ">>> Checking KVM..."
if [ ! -e /dev/kvm ]; then
    echo "ERROR: /dev/kvm not found. KVM is required."
    exit 1
fi
# Ensure athanor user can access KVM
usermod -a -G kvm athanor 2>/dev/null || true

echo ""
echo "=== VPS setup complete ==="
echo ""
echo "Next steps:"
echo "  1. Update /etc/athanor/env with KERNEL_PATH=/var/lib/athanor/vmlinux"
echo "  2. Restart athanor: systemctl restart athanor"
echo "  3. Push to your repo to test"
