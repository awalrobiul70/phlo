# Phlo

Phlo is a personal, portable local PHP environment for Windows and Linux.

It keeps the web server, PHP, Composer, databases, and your sites in one folder. The dashboard is at [http://127.0.0.1:7700](http://127.0.0.1:7700). The same page has a Guide menu with these steps.

## What you get

- A site at `home/{name}/public_html` opens as `http://{name}.test/`
- A folder at `home/{name}/subdomain/{sub}` opens as `http://{sub}.{name}.test/`
- Each folder can use its own PHP version, or the default from Settings
- A custom host such as `myinventory.test` (it must end with the TLD from Settings, usually `.test`)
- MariaDB as the MySQL server (`root`, empty password) and phpMyAdmin at `phpmyadmin.test`
- Optional PostgreSQL (`postgres`, no password) and MongoDB
- Optional Redis, Memcached, and Mailpit (Mailpit stores mail on this PC; it does not send it)
- Node.js, Python, Java, and C++ on the project Terminal. Phlo does not start apps written in those languages
- The Docker command, which uses a Docker engine you install yourself
- Optional HTTPS with a certificate Phlo creates for this PC

## Before you start

- Windows or Linux, 64-bit
- On Windows, the [Microsoft Visual C++ runtime (2015–2022 x64)](https://learn.microsoft.com/en-us/cpp/windows/latest-supported-vc-redist). If it is missing, the dashboard shows an Install button
- On Windows, keep Phlo in a folder whose path uses only English letters, numbers, spaces, `-`, or `_`, such as `D:\Phlo`. A path with Bangla letters or other non-English characters will not start
- Permission to use the HTTP port and, the first time, to write site names into the hosts file

## Start and stop

| | |
| --- | --- |
| Windows | Double-click `Phlo.cmd`, or the Phlo icon |
| Linux | Run `chmod +x Phlo.sh` once, then `./Phlo.sh` |

Start turns on the web server, the PHP versions your sites need, and each database or tool that is switched on in Settings. Stop turns those off and leaves the dashboard open.

On Windows, Minimize leaves Phlo running. Close leaves the dashboard running while the server is on. When the server is already stopped, Close also stops the dashboard. If port 7700 is already in use, the window asks for another port and leaves the other program running.

On Linux, closing the window that you started with `Phlo.sh` stops Phlo.

## Add a site

1. Open **Projects**.
2. Type a name such as `inventory` and press **Add project**.
3. Phlo creates `home/inventory/public_html` and serves it as `inventory.test`. Add a subdomain named `shop` for `shop.inventory.test`.
4. Open the link on the card. When the HTTP port is not 80, the link includes that port, for example `http://inventory.test:83/`.

The first time, Windows may ask for approval to write the hosts file. Phlo only edits the block between `# BEGIN PHLO` and `# END PHLO`.

Download Nginx and at least one PHP version from **Settings → Runtimes** before the site can run. MariaDB, Redis, Memcached, Mailpit, phpMyAdmin, Composer, PostgreSQL, MongoDB, and the toolchains are downloaded from the same page when you want them. PostgreSQL and MongoDB stay off until you turn them on in Settings.

## PHP and the host name

**Change PHP** on the project card sets the version for `public_html` or one subdomain. A version that is not installed can be downloaded from that dialog. An empty choice uses the default PHP in Settings.

**Host** replaces `inventory.test` with a name such as `myinventory.test`. Subdomains follow that name. `phpmyadmin.test` is reserved.

## Databases

MariaDB is the MySQL server. The user is `root` with an empty password. After you download phpMyAdmin, it opens at `phpmyadmin.test`.

PostgreSQL's first start creates the user `postgres` with no password. MongoDB has no user. The PHP extension for MongoDB is downloaded per PHP version from **Settings → MongoDB**.

Copy the `.env` block from **Dashboard** into your project. Phlo does not write that file for you.

HeidiSQL and SQLyog open MariaDB from the MariaDB card. They do not start with Phlo.

Do not start Phlo as administrator or root if you use PostgreSQL. A different PostgreSQL major version needs its own data folder. Phlo stops and tells you what to do, and leaves the current folder as it is.

## Another PC, and port 80

Sites open on this PC. To open them from a phone on the same network, enter this PC's address in **Settings → Web server → Host IP** and save. phpMyAdmin, the dashboard, and the databases stay on `127.0.0.1`.

If another program is using port 80, open the Phlo link that includes Phlo's port. Phlo leaves that program running.

## Mail, cron, and sharing

Mailpit keeps mail on this PC. Point your app at the SMTP port shown on the Dashboard.

On Windows, Cron uses Cronical. If the Cron card says it is not installed, press **Download Cronical**. **Edit jobs** opens `config/custom/cronical.dat`. Each line is Minute, Hour, Day, Month, Weekday, then the command. Remove the `#` from a line to schedule it. **Run test** runs the example PHP command now and writes the output to this month's job log.

**Share** on a project opens one ngrok address for that site after you save an authtoken under **Settings → ngrok**.

## Terminal

**Terminal** on a project opens a shell in that folder. `php` there is the PHP selected for that folder, so `php artisan` uses that version.

**Settings → Toolchains** can put Node.js, Python, Java, C++, and the Docker command on that same Terminal. On Windows, a C++ program built in that Terminal runs there; add `-static` if the `.exe` should run outside it. On Linux, C++ uses the compiler from your distribution (`sudo apt install build-essential`).

## Commands

From the Phlo folder:

```text
bin\phlo.exe start
bin\phlo.exe stop
bin\phlo.exe status
bin\phlo.exe version
bin\phlo.exe php -v
bin\phlo.exe composer --version
bin\phlo.exe mysql
bin\phlo.exe psql
bin\phlo.exe mongosh
bin\phlo.exe node -v
bin\phlo.exe python -V
bin\phlo.exe java -version
bin\phlo.exe g++ --version
bin\phlo.exe docker version
```

On Linux the program is `bin/phlo`. `npm`, `npx`, `pip`, `javac`, and `gcc` work the same way.

`version` prints this Phlo release, for example `Phlo 1.0.0`, and says whether a newer release is available. When one is published, the dashboard shows a notice. **About → Update** installs it in one click. Your sites, databases, and downloaded servers stay on this PC.

## Where your files are

| Folder | What it holds |
| --- | --- |
| `home/` | Your sites |
| `runtime/` | Servers and tools downloaded from the dashboard |
| `data/` | MariaDB databases |
| `data-postgres/` | PostgreSQL databases |
| `data-mongodb/` | MongoDB databases |
| `var/` | Logs, the HTTPS certificate, and the ngrok token |
