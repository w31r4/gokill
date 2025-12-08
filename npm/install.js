const fs = require('fs');
const path = require('path');
const https = require('https');
const { execSync } = require('child_process');

const packageJson = require('./package.json');
const version = packageJson.version;

// Mapping from Node's `process.platform` to Golang's `GOOS`
const PLATFORM_MAPPING = {
  "darwin": "Darwin",
  "linux": "Linux",
  "win32": "Windows",
};

// Mapping from Node's `process.arch` to Golang's `GOARCH`
const ARCH_MAPPING = {
  "ia32": "i386",
  "x64": "x86_64",
  "arm64": "arm64",
  "arm": "armv6" // Defaulting to armv6 for broad compatibility, adjust if needed
};

function getDownloadUrl() {
  const platform = PLATFORM_MAPPING[process.platform];
  const arch = ARCH_MAPPING[process.arch];

  if (!platform || !arch) {
    console.error(`Unsupported platform: ${process.platform} ${process.arch}`);
    process.exit(1);
  }

  const ext = process.platform === 'win32' ? 'zip' : 'tar.gz';
  const filename = `gokill_${platform}_${arch}.${ext}`;
  return `https://github.com/w31r4/gokill/releases/download/v${version}/${filename}`;
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    https.get(url, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        download(response.headers.location, dest).then(resolve).catch(reject);
        return;
      }
      
      if (response.statusCode !== 200) {
        reject(new Error(`Failed to download: ${response.statusCode}`));
        return;
      }

      response.pipe(file);
      file.on('finish', () => {
        file.close(resolve);
      });
    }).on('error', (err) => {
      fs.unlink(dest, () => reject(err));
    });
  });
}

function extract(source, destDir) {
  if (process.platform === 'win32') {
    // Use PowerShell to unzip on Windows
    execSync(`powershell -command "Expand-Archive -Path '${source}' -DestinationPath '${destDir}' -Force"`);
  } else {
    // Use tar on Linux/macOS
    execSync(`tar -xzf "${source}" -C "${destDir}"`);
  }
}

async function install() {
  const binDir = path.join(__dirname, 'bin');
  if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir);
  }

  const url = getDownloadUrl();
  console.log(`Downloading gokill v${version} from ${url}...`);

  const archivePath = path.join(binDir, 'download.tmp');
  
  try {
    await download(url, archivePath);
    console.log('Download complete. Extracting...');
    
    extract(archivePath, binDir);
    
    // Cleanup
    fs.unlinkSync(archivePath);

    // Ensure binary is executable (Linux/macOS)
    const binName = process.platform === 'win32' ? 'gokill.exe' : 'gokill';
    const binPath = path.join(binDir, binName);
    
    if (process.platform !== 'win32') {
      fs.chmodSync(binPath, 0o755);
    }

    console.log('gokill installed successfully!');
  } catch (error) {
    console.error('Installation failed:', error.message);
    process.exit(1);
  }
}

install();