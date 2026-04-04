# raise-digitalocean

Raise plugin for managing DigitalOcean Droplets via the doctl CLI.

## Prerequisites

- doctl CLI installed and configured (`doctl auth init`)
- Valid DigitalOcean API token

## Usage

```bash
raise up --provider digitalocean -f droplet.yaml mydroplet
raise halt --provider digitalocean mydroplet
raise destroy --provider digitalocean mydroplet
raise ssh --provider digitalocean mydroplet
raise status --provider digitalocean mydroplet
```

## Entity Format

```yaml
_type: amadla.org/entity/infrastructure/cloud@v1.0.0
_body:
  region: nyc1
  size: s-1vcpu-1gb
  image: ubuntu-22-04-x64
  ssh_keys:
    - "12345678"
  tags:
    - web
    - production
  ssh:
    user: root
    private_key: ~/.ssh/id_rsa
```

## License

Apache 2.0
