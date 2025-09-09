# GitHub Actions Setup

## Required Secrets

To use the Docker build and push workflow, you need to configure the following secrets in your GitHub repository:

### 1. Go to Repository Settings
- Navigate to your repository on GitHub
- Click on **Settings** tab
- Click on **Secrets and variables** → **Actions**

### 2. Add Required Secrets

#### `QUAY_USERNAME`
- **Description**: Your Quay.io username
- **Value**: Your Quay.io username (e.g., `benarmosec`)

#### `QUAY_TOKEN`
- **Description**: Your Quay.io authentication token
- **Value**: Generate a token from Quay.io:
  1. Go to [Quay.io](https://quay.io)
  2. Click on your profile → **Account Settings**
  3. Go to **Applications** tab
  4. Click **Create Application**
  5. Give it a name (e.g., "GitHub Actions")
  6. Select **Generate Token**
  7. Copy the generated token

### 3. Alternative: Docker Hub

If you prefer to use Docker Hub instead of Quay.io, update the workflow file:

```yaml
- name: Log in to Container Registry
  uses: docker/login-action@v3
  with:
    username: ${{ secrets.DOCKER_USERNAME }}
    password: ${{ secrets.DOCKER_TOKEN }}
```

And add these secrets instead:
- `DOCKER_USERNAME`: Your Docker Hub username
- `DOCKER_TOKEN`: Your Docker Hub access token

## Usage

### Manual Trigger
1. Go to **Actions** tab in your repository
2. Select **Build and Push Docker Images** workflow
3. Click **Run workflow**
4. Configure the inputs:
   - **Server tag**: Tag for server image (default: `latest`)
   - **Client tag**: Tag for client image (default: `latest`)
   - **Registry**: Docker registry (default: `quay.io/benarmosec/curing`)

### Example Usage
```bash
# Default tags
Server: quay.io/benarmosec/curing-server:latest
Client: quay.io/benarmosec/curing-client:latest

# Custom tags
Server: quay.io/benarmosec/curing-server:v1.2.3
Client: quay.io/benarmosec/curing-client:v1.2.3
```

## Features

- ✅ **Multi-architecture builds** (AMD64 + ARM64)
- ✅ **Manual workflow dispatch** with customizable inputs
- ✅ **GitHub Actions cache** for faster builds
- ✅ **Automatic metadata extraction** with multiple tags
- ✅ **Build summary** with usage instructions
- ✅ **Secure secret management**
