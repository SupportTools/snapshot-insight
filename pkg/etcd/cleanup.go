package etcd

import (
	"fmt"
	"os/exec"
)

// CleanupEtcd removes the etcd Docker container and any temporary resources created during the restore process.
func CleanupEtcd(containerName string) error {
	fmt.Printf("Stopping and removing etcd container: %s...\n", containerName)

	// Stop and remove the Docker container
	cmd := exec.Command("docker", "rm", "-f", containerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clean up etcd container %s: %v", containerName, err)
	}

	fmt.Println("Etcd container cleaned up successfully.")
	return nil
}

// CleanupKubeAPIServer removes the kube-apiserver Docker container.
func CleanupKubeAPIServer(containerName string) error {
	fmt.Printf("Stopping and removing kube-apiserver container: %s...\n", containerName)

	// Stop and remove the Docker container
	cmd := exec.Command("docker", "rm", "-f", containerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clean up kube-apiserver container %s: %v", containerName, err)
	}

	fmt.Println("Kube-apiserver container cleaned up successfully.")
	return nil
}

// CleanupVolume removes a specified Docker volume.
func CleanupVolume(volumeName string) error {
	fmt.Printf("Removing Docker volume: %s...\n", volumeName)

	cmd := exec.Command("docker", "volume", "rm", volumeName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clean up Docker volume %s: %v", volumeName, err)
	}

	fmt.Println("Docker volume cleaned up successfully.")
	return nil
}
