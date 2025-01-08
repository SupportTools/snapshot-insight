package etcd

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// StartEtcdServer starts an etcd server using the specified Docker volume and host networking.
func StartEtcdServer(volumeName, containerName string) (string, error) {
	// Resolve the host's primary IP address
	hostIP, err := getHostIPAddress()
	if err != nil {
		return "", fmt.Errorf("failed to resolve host IP address: %v", err)
	}

	// Build the advertise-client-urls value
	advertiseURLs := fmt.Sprintf("http://127.0.0.1:2379,http://%s:2379", hostIP)

	// Remove existing container if it exists
	fmt.Printf("Removing existing etcd container: %s (if running)...\n", containerName)
	cmdRemove := exec.Command("docker", "rm", "-f", containerName)
	cmdRemove.Stdout = os.Stdout
	cmdRemove.Stderr = os.Stderr
	_ = cmdRemove.Run() // Ignore errors if the container doesn't exist

	// Log the details of the action being performed
	fmt.Printf("Starting etcd server using Docker volume: %s...\n", volumeName)

	// Build the command
	cmdRun := exec.Command("docker", "run", "-d", "--name", containerName,
		"--network", "host", // Use host network mode
		"-v", fmt.Sprintf("%s:/etcd-data", volumeName), // Use Docker volume
		"quay.io/coreos/etcd:v3.5.7", // Image
		"/usr/local/bin/etcd", "--name=restored-etcd",
		"--data-dir=/etcd-data",
		"--advertise-client-urls="+advertiseURLs,
		"--listen-client-urls=http://0.0.0.0:2379",
		"--listen-peer-urls=http://0.0.0.0:2380")

	// Log the full command for debugging
	fmt.Printf("Executing command: %s\n", strings.Join(cmdRun.Args, " "))

	// Capture and log the output of the command
	output, err := cmdRun.CombinedOutput()
	fmt.Printf("Command output:\n%s\n", string(output))
	if err != nil {
		return "", fmt.Errorf("failed to start etcd server: %v", err)
	}

	fmt.Println("Etcd server started successfully and is listening on host ports.")
	return hostIP, nil
}

// StartKubeAPIServer starts a kube-apiserver using the specified etcd endpoint and Docker volume for certificates.
func StartKubeAPIServer(etcdEndpoint, containerName, volumeName, hostIP, outputDir string) error {
	// Remove existing kube-apiserver container if it exists
	fmt.Printf("Removing existing kube-apiserver container: %s (if running)...\n", containerName)
	cmdRemove := exec.Command("docker", "rm", "-f", containerName)
	cmdRemove.Stdout = os.Stdout
	cmdRemove.Stderr = os.Stderr
	_ = cmdRemove.Run() // Ignore errors if the container doesn't exist

	if etcdEndpoint == "" {
		return fmt.Errorf("etcd endpoint is required to start kube-apiserver")
	}

	// Create Docker volume for certificates if it doesn't exist
	fmt.Printf("Creating Docker volume for certificates: %s...\n", volumeName)
	cmdVolumeCreate := exec.Command("docker", "volume", "create", volumeName)
	cmdVolumeCreate.Stdout = os.Stdout
	cmdVolumeCreate.Stderr = os.Stderr
	if err := cmdVolumeCreate.Run(); err != nil {
		return fmt.Errorf("failed to create Docker volume: %v", err)
	}

	// Paths inside the Docker volume
	volumeCertDir := "/certs"
	caCertPath := filepath.Join(volumeCertDir, "ca.crt")
	caKeyPath := filepath.Join(volumeCertDir, "ca.key")

	// Generate certificates and keys in the Docker volume
	fmt.Println("Generating certificates and keys in Docker volume...")
	if err := GenerateSelfSignedCAInVolume(volumeName, volumeCertDir, hostIP); err != nil {
		return fmt.Errorf("error generating self-signed CA: %v", err)
	}

	// Path to encryption configuration
	encryptionConfigPath := "./encryption-config.json"
	if _, err := os.Stat(encryptionConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("encryption configuration file not found at %s", encryptionConfigPath)
	}

	// Start kube-apiserver with certificates from the Docker volume
	fmt.Printf("Starting kube-apiserver container: %s...\n", containerName)
	cmdRun := exec.Command("docker", "run", "-d", "--name", containerName,
		"--network", "host", // Use host network mode
		"-v", fmt.Sprintf("%s:%s", volumeName, volumeCertDir), // Mount Docker volume
		"-v", fmt.Sprintf("%s:/etc/kubernetes/encryption-config.json", encryptionConfigPath), // Mount encryption config
		"k8s.gcr.io/kube-apiserver:v1.27.1",
		"/usr/local/bin/kube-apiserver",
		"--etcd-servers="+etcdEndpoint,
		"--service-cluster-ip-range=10.96.0.0/12",
		"--allow-privileged=true",
		"--anonymous-auth=true",
		"--advertise-address=0.0.0.0",
		"--service-account-signing-key-file="+caKeyPath,
		"--service-account-issuer=https://kubernetes.default.svc.cluster.local",
		"--service-account-key-file="+caCertPath,
		"--tls-cert-file="+caCertPath,
		"--tls-private-key-file="+caKeyPath,
		"--client-ca-file="+caCertPath,
		"--tls-cert-file="+caCertPath,
		"--tls-private-key-file="+caKeyPath,
		"--v=2") // Verbose logging level
	cmdRun.Stdout = os.Stdout
	cmdRun.Stderr = os.Stderr
	if err := cmdRun.Run(); err != nil {
		return fmt.Errorf("failed to start kube-apiserver: %v", err)
	}

	fmt.Println("Kube-apiserver started successfully and is listening on port 8080.")
	return nil
}

// GenerateSelfSignedCAWithSAN creates a self-signed CA certificate, private key, client certificate, and client key.
func GenerateSelfSignedCAWithSAN(caCertPath, caKeyPath, clientCertPath, clientKeyPath, hostIP string) error {
	// Generate the CA private key
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate CA private key: %v", err)
	}

	// Create the CA certificate template
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Kubernetes"},
			CommonName:   "Kubernetes CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP(hostIP)},
	}

	// Create the CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPriv.PublicKey, caPriv)
	if err != nil {
		return fmt.Errorf("failed to create CA certificate: %v", err)
	}

	// Write the CA certificate and private key to files
	if err := writePEMFile(caCertPath, "CERTIFICATE", caCertDER); err != nil {
		return fmt.Errorf("failed to write CA certificate: %v", err)
	}

	caPrivBytes, err := x509.MarshalECPrivateKey(caPriv)
	if err != nil {
		return fmt.Errorf("failed to marshal CA private key: %v", err)
	}

	if err := writePEMFile(caKeyPath, "EC PRIVATE KEY", caPrivBytes); err != nil {
		return fmt.Errorf("failed to write CA private key: %v", err)
	}

	// Generate the client private key
	clientPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate client private key: %v", err)
	}

	// Create the client certificate template
	clientTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Kubernetes"},
			CommonName:   "Kubernetes Client",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP(hostIP)},
	}

	// Sign the client certificate with the CA
	clientCertDER, err := x509.CreateCertificate(rand.Reader, &clientTemplate, &caTemplate, &clientPriv.PublicKey, caPriv)
	if err != nil {
		return fmt.Errorf("failed to create client certificate: %v", err)
	}

	// Write the client certificate and private key to files
	if err := writePEMFile(clientCertPath, "CERTIFICATE", clientCertDER); err != nil {
		return fmt.Errorf("failed to write client certificate: %v", err)
	}

	clientPrivBytes, err := x509.MarshalECPrivateKey(clientPriv)
	if err != nil {
		return fmt.Errorf("failed to marshal client private key: %v", err)
	}

	if err := writePEMFile(clientKeyPath, "EC PRIVATE KEY", clientPrivBytes); err != nil {
		return fmt.Errorf("failed to write client private key: %v", err)
	}

	fmt.Printf("CA and client certificates generated successfully:\n- CA Cert: %s\n- Client Cert: %s\n", caCertPath, clientCertPath)
	return nil
}

// GenerateClientCert creates a client certificate and private key signed by the provided CA.
func GenerateClientCert(caCertPath, caKeyPath, clientCertPath, clientKeyPath, hostIP string) error {
	// Read CA certificate
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate: %v", err)
	}

	// Parse CA certificate
	caCertBlock, _ := pem.Decode(caCertPEM)
	if caCertBlock == nil {
		return fmt.Errorf("failed to decode CA certificate PEM")
	}
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %v", err)
	}

	// Read CA private key
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read CA private key: %v", err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		return fmt.Errorf("failed to decode CA private key PEM")
	}
	caKey, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA private key: %v", err)
	}

	// Generate client key pair
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate client private key: %v", err)
	}

	// Create client certificate template
	clientCertTemplate := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			Organization: []string{"Kubernetes"},
			CommonName:   "Kubernetes Client",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP(hostIP)},
	}

	// Sign client certificate
	clientCertDER, err := x509.CreateCertificate(rand.Reader, &clientCertTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("failed to create client certificate: %v", err)
	}

	// Write client certificate
	clientCertFile, err := os.Create(clientCertPath)
	if err != nil {
		return fmt.Errorf("failed to create client certificate file: %v", err)
	}
	defer clientCertFile.Close()
	if err := pem.Encode(clientCertFile, &pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER}); err != nil {
		return fmt.Errorf("failed to encode client certificate to PEM: %v", err)
	}

	// Marshal client private key
	clientKeyBytes, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		return fmt.Errorf("failed to marshal client private key: %v", err)
	}

	// Write client private key
	clientKeyFile, err := os.Create(clientKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create client private key file: %v", err)
	}
	defer clientKeyFile.Close()
	if err := pem.Encode(clientKeyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyBytes}); err != nil {
		return fmt.Errorf("failed to encode client private key to PEM: %v", err)
	}

	fmt.Printf("Client certificate and key generated at: %s, %s\n", clientCertPath, clientKeyPath)
	return nil
}

// writePEMFile writes data to a PEM file.
func writePEMFile(path, blockType string, data []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return pem.Encode(file, &pem.Block{Type: blockType, Bytes: data})
}

// getHostIPAddress retrieves the primary IP address of the host.
func getHostIPAddress() (string, error) {
	// Execute the hostname -I command to get the IP addresses
	cmd := exec.Command("hostname", "-I")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to execute hostname -I: %v", err)
	}

	// Parse the first IP address from the output
	ipAddresses := strings.Fields(stdout.String())
	if len(ipAddresses) == 0 {
		return "", fmt.Errorf("no IP addresses found in hostname -I output")
	}

	return ipAddresses[0], nil
}

// GenerateSelfSignedCAInVolume generates a self-signed CA, client certificate, and client key with SAN, and stores them in a Docker volume.
func GenerateSelfSignedCAInVolume(volumeName, volumeCertDir, hostIP string) error {
	// Create a temporary directory for the certificates
	tempDir, err := os.MkdirTemp("", "kube-apiserver-certs")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Paths for the CA cert and key
	caCertPath := filepath.Join(tempDir, "ca.crt")
	caKeyPath := filepath.Join(tempDir, "ca.key")
	clientCertPath := filepath.Join(tempDir, "client.crt")
	clientKeyPath := filepath.Join(tempDir, "client.key")

	// Generate the self-signed CA and client credentials
	fmt.Println("Generating self-signed CA and client certificates...")
	if err := GenerateSelfSignedCAWithSAN(caCertPath, caKeyPath, clientCertPath, clientKeyPath, hostIP); err != nil {
		return fmt.Errorf("failed to generate self-signed CA and client certificates: %v", err)
	}

	// Copy certificates and keys into the Docker volume
	fmt.Printf("Copying certificates and keys into Docker volume: %s...\n", volumeName)
	cmdCopyCert := exec.Command("docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:%s", volumeName, volumeCertDir),
		"-v", fmt.Sprintf("%s:/tmp/certs", tempDir),
		"alpine", "sh", "-c", "cp /tmp/certs/* /certs/")
	cmdCopyCert.Stdout = os.Stdout
	cmdCopyCert.Stderr = os.Stderr
	if err := cmdCopyCert.Run(); err != nil {
		return fmt.Errorf("failed to copy certificates and keys into Docker volume: %v", err)
	}

	fmt.Println("Certificates and keys successfully stored in Docker volume.")
	return nil
}

// GenerateKubeconfig creates a kubeconfig file using certs copied from the kube-apiserver container.
func GenerateKubeconfig(kubeconfigPath, serverURL, containerName string) error {
	const kubeconfigTemplate = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: {{ .ServerURL }}
    certificate-authority-data: {{ .CACertData }}
  name: kubernetes
contexts:
- context:
    cluster: kubernetes
    user: admin
  name: kubernetes
current-context: kubernetes
users:
- name: admin
  user:
    client-certificate-data: {{ .ClientCertData }}
    client-key-data: {{ .ClientKeyData }}
`

	// Create a temporary directory to copy certs from the container
	tempDir, err := os.MkdirTemp("", "kubeconfig-certs")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Paths inside the container
	caCertContainerPath := "/certs/ca.crt"
	clientCertContainerPath := "/certs/client.crt"
	clientKeyContainerPath := "/certs/client.key"

	// Local paths to store the copied certs
	caCertLocalPath := filepath.Join(tempDir, "ca.crt")
	clientCertLocalPath := filepath.Join(tempDir, "client.crt")
	clientKeyLocalPath := filepath.Join(tempDir, "client.key")

	// Copy certificates from the kube-apiserver container
	if err := copyFileFromContainer(containerName, caCertContainerPath, caCertLocalPath); err != nil {
		return fmt.Errorf("failed to copy CA certificate from container: %v", err)
	}
	if err := copyFileFromContainer(containerName, clientCertContainerPath, clientCertLocalPath); err != nil {
		return fmt.Errorf("failed to copy client certificate from container: %v", err)
	}
	if err := copyFileFromContainer(containerName, clientKeyContainerPath, clientKeyLocalPath); err != nil {
		return fmt.Errorf("failed to copy client key from container: %v", err)
	}

	// Read and encode the certs
	caCertBytes, err := os.ReadFile(caCertLocalPath)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate: %v", err)
	}
	clientCertBytes, err := os.ReadFile(clientCertLocalPath)
	if err != nil {
		return fmt.Errorf("failed to read client certificate: %v", err)
	}
	clientKeyBytes, err := os.ReadFile(clientKeyLocalPath)
	if err != nil {
		return fmt.Errorf("failed to read client key: %v", err)
	}

	// Base64 encode the certs
	data := struct {
		ServerURL      string
		CACertData     string
		ClientCertData string
		ClientKeyData  string
	}{
		ServerURL:      serverURL,
		CACertData:     base64.StdEncoding.EncodeToString(caCertBytes),
		ClientCertData: base64.StdEncoding.EncodeToString(clientCertBytes),
		ClientKeyData:  base64.StdEncoding.EncodeToString(clientKeyBytes),
	}

	// Write the kubeconfig file
	file, err := os.Create(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create kubeconfig file: %v", err)
	}
	defer file.Close()

	tmpl, err := template.New("kubeconfig").Parse(kubeconfigTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse kubeconfig template: %v", err)
	}

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to write kubeconfig file: %v", err)
	}

	fmt.Printf("Kubeconfig generated at: %s\n", kubeconfigPath)
	return nil
}

// copyFileFromContainer copies a file from a Docker container to a local path.
func copyFileFromContainer(containerName, containerPath, localPath string) error {
	cmd := exec.Command("docker", "cp", fmt.Sprintf("%s:%s", containerName, containerPath), localPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy file from container: %v", err)
	}
	return nil
}
