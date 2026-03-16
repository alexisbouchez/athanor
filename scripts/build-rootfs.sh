#!/bin/bash
set -euo pipefail

# Build a rootfs image for Athanor CI microVMs.
# Must be run as root on an Ubuntu/Debian host.
# Usage: sudo ./scripts/build-rootfs.sh

ROOTFS_DIR="/tmp/athanor-rootfs"
IMAGE_PATH="/var/lib/athanor/rootfs.ext4"
IMAGE_SIZE_MB=4096
SSH_KEY_PATH="/var/lib/athanor/vm-ssh-key"
GO_VERSION="1.26.1"

echo "=== Building Athanor CI rootfs ==="

# Clean up
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"

# 1. Debootstrap minimal Ubuntu 24.04
echo ">>> Debootstrap noble..."
debootstrap --arch=amd64 noble "$ROOTFS_DIR" http://archive.ubuntu.com/ubuntu

# 2. Mount virtual filesystems
mount --bind /dev "$ROOTFS_DIR/dev"
mount --bind /dev/pts "$ROOTFS_DIR/dev/pts"
mount -t proc proc "$ROOTFS_DIR/proc"
mount -t sysfs sysfs "$ROOTFS_DIR/sys"

cleanup() {
    echo ">>> Cleaning up mounts..."
    umount -l "$ROOTFS_DIR/dev/pts" 2>/dev/null || true
    umount -l "$ROOTFS_DIR/dev" 2>/dev/null || true
    umount -l "$ROOTFS_DIR/proc" 2>/dev/null || true
    umount -l "$ROOTFS_DIR/sys" 2>/dev/null || true
}
trap cleanup EXIT

# 3. Configure inside chroot
cat > "$ROOTFS_DIR/setup.sh" << 'CHROOT_SCRIPT'
#!/bin/bash
set -euo pipefail

# Configure apt
cat > /etc/apt/sources.list << 'APT'
deb http://archive.ubuntu.com/ubuntu noble main restricted universe
deb http://archive.ubuntu.com/ubuntu noble-updates main restricted universe
deb http://security.ubuntu.com/ubuntu noble-security main restricted universe
APT

apt-get update
apt-get install -y --no-install-recommends \
    openssh-server \
    git \
    bash \
    curl \
    ca-certificates \
    iproute2 \
    iputils-ping \
    dnsutils \
    build-essential \
    sudo

# Install Go
curl -sL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
tar -C /usr/local -xzf /tmp/go.tar.gz
rm /tmp/go.tar.gz
ln -sf /usr/local/go/bin/go /usr/local/bin/go
ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

# Configure SSH
mkdir -p /root/.ssh
chmod 700 /root/.ssh
sed -i 's/#PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config
sed -i 's/#PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
sed -i 's/#PubkeyAuthentication.*/PubkeyAuthentication yes/' /etc/ssh/sshd_config
# Generate host keys
ssh-keygen -A

# Configure networking — will be set via kernel cmdline
cat > /etc/network/interfaces << 'NETCFG'
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet manual
NETCFG

# Set up DNS
echo "nameserver 8.8.8.8" > /etc/resolv.conf

# Set hostname
echo "athanor-vm" > /etc/hostname

# Workspace mount point (virtiofs)
mkdir -p /workspace
echo "workspace /workspace virtiofs defaults 0 0" >> /etc/fstab

# Auto-start SSH on boot
systemctl enable ssh

# Set root password (disabled, key only)
passwd -l root

# Clean up
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/*

echo "Chroot setup complete"
CHROOT_SCRIPT

# Pass Go version to chroot script
sed -i "s/\${GO_VERSION}/$GO_VERSION/" "$ROOTFS_DIR/setup.sh"
chmod +x "$ROOTFS_DIR/setup.sh"
chroot "$ROOTFS_DIR" /setup.sh
rm "$ROOTFS_DIR/setup.sh"

# 4. Generate SSH key pair
echo ">>> Generating SSH key pair..."
mkdir -p "$(dirname "$SSH_KEY_PATH")"
if [ ! -f "$SSH_KEY_PATH" ]; then
    ssh-keygen -t ed25519 -f "$SSH_KEY_PATH" -N "" -C "athanor-vm"
fi
cp "${SSH_KEY_PATH}.pub" "$ROOTFS_DIR/root/.ssh/authorized_keys"
chmod 600 "$ROOTFS_DIR/root/.ssh/authorized_keys"

# 5. Configure init (systemd)
# Ensure serial console works
mkdir -p "$ROOTFS_DIR/etc/systemd/system/serial-getty@ttyS0.service.d"
cat > "$ROOTFS_DIR/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf" << 'SERIAL'
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I 115200 linux
SERIAL

# Unmount before packing
cleanup
trap - EXIT

# 6. Pack into ext4 image
echo ">>> Creating ext4 image ($IMAGE_SIZE_MB MB)..."
mkdir -p "$(dirname "$IMAGE_PATH")"
dd if=/dev/zero of="$IMAGE_PATH" bs=1M count="$IMAGE_SIZE_MB" status=progress
mkfs.ext4 -F "$IMAGE_PATH"

MOUNT_POINT="/tmp/athanor-rootfs-mount"
mkdir -p "$MOUNT_POINT"
mount -o loop "$IMAGE_PATH" "$MOUNT_POINT"
cp -a "$ROOTFS_DIR/." "$MOUNT_POINT/"
umount "$MOUNT_POINT"
rmdir "$MOUNT_POINT"

# Clean up build dir
rm -rf "$ROOTFS_DIR"

echo ">>> rootfs image created at $IMAGE_PATH"
echo ">>> SSH private key at $SSH_KEY_PATH"
echo ""
echo "=== Done ==="
