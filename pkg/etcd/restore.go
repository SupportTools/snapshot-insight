package etcd

import (
	"fmt"
	"os"
	"os/exec"
)

// RestoreEtcdSnapshot restores an etcd snapshot using etcdutl directly within a Docker container.
func RestoreEtcdSnapshot(snapshotPath, containerName, volumeName string) error {
	// Validate snapshot existence
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		return fmt.Errorf("snapshot file not found: %s", snapshotPath)
	}

	// Pull etcd Docker image
	fmt.Println("Pulling etcd Docker image...")
	cmdPull := exec.Command("docker", "pull", "quay.io/coreos/etcd:v3.5.7")
	cmdPull.Stdout = os.Stdout
	cmdPull.Stderr = os.Stderr
	if err := cmdPull.Run(); err != nil {
		return fmt.Errorf("failed to pull etcd Docker image: %v", err)
	}

	// Remove existing container if it exists
	fmt.Printf("Removing existing container: %s (if running)...\n", containerName)
	cmdRemove := exec.Command("docker", "rm", "-f", containerName)
	cmdRemove.Stdout = os.Stdout
	cmdRemove.Stderr = os.Stderr
	_ = cmdRemove.Run() // Ignore errors if the container doesn't exist

	// Remove existing Docker volume if it exists
	fmt.Printf("Removing existing Docker volume: %s (if exists)...\n", volumeName)
	cmdVolumeRemove := exec.Command("docker", "volume", "rm", volumeName)
	cmdVolumeRemove.Stdout = os.Stdout
	cmdVolumeRemove.Stderr = os.Stderr
	_ = cmdVolumeRemove.Run() // Ignore errors if the volume doesn't exist

	// Create a Docker volume for etcd data
	fmt.Printf("Creating Docker volume: %s...\n", volumeName)
	cmdVolumeCreate := exec.Command("docker", "volume", "create", volumeName)
	cmdVolumeCreate.Stdout = os.Stdout
	cmdVolumeCreate.Stderr = os.Stderr
	if err := cmdVolumeCreate.Run(); err != nil {
		return fmt.Errorf("failed to create Docker volume: %v", err)
	}

	// Run the etcdutl snapshot restore command
	fmt.Printf("Restoring snapshot: %s into Docker volume: %s...\n", snapshotPath, volumeName)
	cmdRestore := exec.Command("docker", "run", "--rm", "--name", containerName,
		"-v", fmt.Sprintf("%s:/snapshot.db", snapshotPath), // Mount snapshot file
		"-v", fmt.Sprintf("%s:/etcd-data", volumeName), // Use Docker volume for output
		"quay.io/coreos/etcd:v3.5.7",                    // Image
		"/usr/local/bin/etcdutl", "snapshot", "restore", // Command
		"/snapshot.db", "--data-dir=/etcd-data") // Args
	cmdRestore.Stdout = os.Stdout
	cmdRestore.Stderr = os.Stderr
	if err := cmdRestore.Run(); err != nil {
		return fmt.Errorf("failed to restore snapshot: %v", err)
	}

	fmt.Println("Snapshot restored successfully.")
	fmt.Printf("Restored data is available in Docker volume: %s\n", volumeName)
	return nil
}
