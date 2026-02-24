# dc - Basic Auth Quick Reference

## ⚡ Quick Setup

```bash
# 1. Set credentials in prod.env
ADMIN_USERNAME=admin
ADMIN_PASSWORD=your_secure_password

# 2. Build and run
go build -o dcapi
./dcapi
```

## 🔐 Authentication

**All endpoints require Basic Authentication**

Browser: `http://localhost:8080` (will prompt for credentials)
curl: `curl -u admin:password http://localhost:8080/`

## 📋 Common Commands

```bash
# Interactive credential setup
make setup-auth

# Install as systemd service
make install
make enable
make start

# Service management
make status          # Check service status
make logs            # View logs
make restart         # Restart service
make stop            # Stop service

# Update application
make update          # Build, stop, update, start

# Testing
./test-auth.sh       # Test authentication

# Uninstall
make uninstall       # Complete removal
```

## 🔧 Configuration

**prod.env location**: Same directory as executable

Required fields:
```
ADMIN_USERNAME=your_username
ADMIN_PASSWORD=your_password
```

## 🛡️ Security Checklist

- [ ] Set strong password (min 16 chars)
- [ ] Protect prod.env: `chmod 600 prod.env`
- [ ] Use HTTPS in production (reverse proxy)
- [ ] Restrict network access (firewall)
- [ ] Monitor logs for failed attempts

## 🐛 Troubleshooting

| Problem | Solution |
|---------|----------|
| 401 Unauthorized | Check credentials in prod.env |
| Credentials not working | No whitespace, restart service |
| Can't access server | Check logs: `make logs` |
| Service won't start | Ensure credentials are set |

## 📚 Documentation

- [README.md](../README.md) - Full documentation
- [BASIC_AUTH_GUIDE.md](BASIC_AUTH_GUIDE.md) - Detailed guide
- [BASIC_AUTH_IMPLEMENTATION.md](BASIC_AUTH_IMPLEMENTATION.md) - Technical details

## 🔗 Protected Endpoints

All require authentication:
- `/` - Web interface
- `/ws` - WebSocket
- `/api/stacks/` - Stack management
- `/api/containers/` - Container API
- `/api/enrich/` - YAML enrichment
- `/thumbnail/` - Thumbnails

## 💡 Tips

**Generate strong password:**
```bash
openssl rand -base64 24
```

**Test credentials:**
```bash
./test-auth.sh
```

**View current username:**
```bash
grep '^ADMIN_USERNAME=' prod.env
```

**Update and restart:**
```bash
make setup-auth && make restart
```

