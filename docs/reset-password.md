# Resetting a password

Kivo sends no email, so a forgotten password is reset by whoever runs the server, from the command line. It works for any account, including admins, without stopping the server.

```sh
docker exec -it kivo kivo reset-password alice
```

It asks for the new password twice, without showing it. To script it, give the password on stdin instead, as one line:

```sh
echo 'the new password' | docker exec -i kivo kivo reset-password alice
```

The password is never an argument, since shell history and `ps` would keep it. Passwords must be 8 to 1024 bytes.

This signs the account out on every device. API keys keep working.

Run as root (the `docker exec` default), the command switches to the database owner (`PUID:PGID`), so nothing in `/data` ends up owned by root.

Outside Docker, run `kivo reset-password [-config path] <username>` with the server's `server.toml` or `KIVO_*` environment, so it finds the same database.
