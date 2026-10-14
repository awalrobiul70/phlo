<?php
header('Content-Type: text/html; charset=utf-8');

$host = (string) ($_SERVER['SERVER_NAME'] ?? '');
$port = (string) ($_SERVER['SERVER_PORT'] ?? '');
$https = !empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off';
$scheme = $https ? 'https' : 'http';
$hidePort = ($scheme === 'http' && $port === '80') || ($scheme === 'https' && $port === '443');
$url = $scheme . '://' . $host . ($port !== '' && !$hidePort ? ':' . $port : '') . '/';
$root = (string) ($_SERVER['DOCUMENT_ROOT'] ?? '');
$software = (string) ($_SERVER['SERVER_SOFTWARE'] ?? '');
$system = PHP_OS . (PHP_INT_SIZE === 8 ? ' · 64-bit' : ' · 32-bit');
$when = date('Y-m-d H:i');

function phlo_h($value)
{
    return htmlspecialchars((string) $value, ENT_QUOTES, 'UTF-8');
}
?>
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="robots" content="noindex">
  <title><?php echo $host !== '' ? phlo_h($host) : 'Phlo'; ?></title>
  <style>
    :root {
      --ink: #17241e;
      --muted: #5d6b63;
      --leaf: #0f6a45;
      --leaf-deep: #0c3d2c;
      --paper: #f3efe7;
      --card: #fffcf8;
      --line: rgba(23, 36, 30, 0.1);
      --gold: #e0b03a;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      color: var(--ink);
      background:
        radial-gradient(900px 420px at 100% -10%, rgba(15, 106, 69, 0.16), transparent 60%),
        radial-gradient(700px 360px at -10% 0%, rgba(196, 154, 74, 0.16), transparent 55%),
        var(--paper);
      font-family: "Segoe UI", "Aptos", sans-serif;
      line-height: 1.5;
    }
    main {
      width: min(44rem, calc(100% - 2rem));
      margin: 0 auto;
      padding: 2.4rem 0 3rem;
    }
    .card {
      background: var(--card);
      border: 1px solid var(--line);
      border-radius: 22px;
      box-shadow: 0 18px 50px rgba(23, 36, 30, 0.08);
      padding: 1.6rem 1.5rem 1.4rem;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 0.9rem;
    }
    .logo { width: 3.4rem; height: 3.4rem; flex: none; }
    .mark {
      margin: 0;
      font-size: 0.78rem;
      letter-spacing: 0.16em;
      text-transform: uppercase;
      color: var(--leaf);
      font-weight: 700;
    }
    h1 {
      margin: 0.1rem 0 0;
      font-size: clamp(1.7rem, 4vw, 2.15rem);
      letter-spacing: -0.04em;
      line-height: 1.05;
    }
    .host {
      margin: 1.15rem 0 0;
      font-size: clamp(1.35rem, 3vw, 1.85rem);
      font-weight: 700;
      letter-spacing: -0.03em;
      word-break: break-word;
    }
    .php {
      display: inline-flex;
      margin: 0.7rem 0 0;
      padding: 0.2rem 0.65rem;
      border-radius: 999px;
      background: #e7f3ec;
      color: var(--leaf-deep);
      font-weight: 700;
      font-size: 0.92rem;
    }
    dl {
      display: grid;
      grid-template-columns: 8.5rem 1fr;
      gap: 0.55rem 0.8rem;
      margin: 1.25rem 0 0;
      padding: 1rem 0 0;
      border-top: 1px solid var(--line);
    }
    dt { color: var(--muted); font-size: 0.82rem; padding-top: 0.12rem; }
    dd { margin: 0; font-weight: 650; overflow-wrap: anywhere; }
    a { color: var(--leaf-deep); }
    .about {
      display: grid;
      grid-template-columns: auto 1fr;
      gap: 0.85rem 1rem;
      margin-top: 1rem;
      padding: 1.15rem 1.35rem 1.25rem;
      border-radius: 22px;
      background: var(--card);
      border: 1px solid var(--line);
      box-shadow: 0 18px 50px rgba(23, 36, 30, 0.08);
    }
    .about .logo { width: 2.6rem; height: 2.6rem; }
    .about h2 {
      margin: 0.15rem 0 0.35rem;
      font-size: 1.05rem;
      letter-spacing: -0.02em;
    }
    .about p { margin: 0; color: var(--muted); }
    .about p + p { margin-top: 0.55rem; }
    .note {
      margin: 0.9rem 0 0;
      color: var(--muted);
      font-size: 0.88rem;
    }
    @media (max-width: 640px) {
      main { padding-top: 1.4rem; }
      dl { grid-template-columns: 1fr; gap: 0.15rem; }
      dd { margin-bottom: 0.45rem; }
      .about { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <main>
    <article class="card">
      <div class="brand">
        <?php echo phlo_logo(64); ?>
        <div>
          <p class="mark">Phlo</p>
          <h1>Project is ready</h1>
        </div>
      </div>
      <p class="host"><?php echo phlo_h($host !== '' ? $host : 'This folder'); ?></p>
      <p class="php">PHP <?php echo phlo_h(PHP_VERSION); ?></p>
      <dl>
        <dt>Address</dt>
        <dd><a href="<?php echo phlo_h($url); ?>"><?php echo phlo_h($url); ?></a></dd>
        <dt>Document root</dt>
        <dd><?php echo phlo_h($root !== '' ? $root : '—'); ?></dd>
        <dt>Server</dt>
        <dd><?php echo phlo_h($software !== '' ? $software : '—'); ?></dd>
        <dt>System</dt>
        <dd><?php echo phlo_h($system); ?></dd>
        <dt>PHP interface</dt>
        <dd><?php echo phlo_h(PHP_SAPI); ?></dd>
        <dt>Time</dt>
        <dd><?php echo phlo_h($when); ?></dd>
      </dl>
      <p class="note">Replace <code>index.php</code> in this folder when your own site is ready. The Laravel button on the project card does that for you.</p>
    </article>
    <section class="about">
      <?php echo phlo_logo(48); ?>
      <div>
        <h2>About Phlo</h2>
        <p>Phlo is a personal, portable local environment for PHP on Windows and Linux. It keeps Nginx, your PHP builds, Composer, and this project folder together, and serves the folder as the host above.</p>
        <p>Each <code>public_html</code> and subdomain can use its own PHP version. The dashboard, on <code>127.0.0.1</code>, is where you change that version, the host name, and the HTTP port.</p>
      </div>
    </section>
  </main>
</body>
</html>
<?php
function phlo_logo($size)
{
    $size = (int) $size;
    return '<svg class="logo" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="' . $size . '" height="' . $size . '" role="img" aria-label="Phlo">'
        . '<defs><linearGradient id="phlo-paper-' . $size . '" x1="8" y1="4" x2="56" y2="62" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#fffdf8"/><stop offset="1" stop-color="#e5f5ec"/></linearGradient>'
        . '<linearGradient id="phlo-ink-' . $size . '" x1="16" y1="8" x2="54" y2="52" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#1a9a64"/><stop offset="1" stop-color="#073224"/></linearGradient></defs>'
        . '<rect width="64" height="64" rx="16" fill="url(#phlo-paper-' . $size . ')"/>'
        . '<rect x="2" y="2" width="60" height="60" rx="14.2" fill="none" stroke="#0d5c3c" stroke-opacity="0.16" stroke-width="1.4"/>'
        . '<path d="M22 50.5 V20.2 C22 14 28.6 12.2 35 12.2 C46.8 12.2 52.4 18 52.4 26.6 C52.4 36.2 44.6 41.2 34 40.2 C28.6 39.6 25.2 36.8 24 33.4" fill="none" stroke="url(#phlo-ink-' . $size . ')" stroke-width="6.5" stroke-linecap="round"/>'
        . '<circle cx="24" cy="33.4" r="3.35" fill="#e0b03a"/>'
        . '<circle cx="22.9" cy="32.3" r="1.05" fill="#fff4cc"/></svg>';
}
