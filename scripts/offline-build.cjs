// Optional fallback when npm registry is unavailable. Reads an existing dependency
// directory without modifying it. Normal installs use Vite via npm run build.
const fs = require('node:fs');
const path = require('node:path');
const deadline = setTimeout(() => { console.error('Offline build timed out. Use npm install and npm run build when network is available.'); process.exit(1); }, 120000);
const modules = path.resolve(process.argv[2] || '');
if (!process.argv[2]) throw new Error('Usage: node scripts/offline-build.cjs /absolute/path/to/node_modules');
const webpack = require(path.join(modules, 'webpack'));
const root = path.resolve(__dirname, '../frontend');
webpack({
  mode: 'production', context: root, entry: './src/main.jsx',
  output: { path: path.join(root, 'dist'), filename: 'assets/app.js', clean: true },
  resolve: { extensions: ['.js', '.jsx', '.json'], modules: [modules] },
  resolveLoader: { modules: [modules] },
  module: { rules: [
    { test: /\.jsx$/, exclude: /node_modules/, use: { loader: path.join(modules, 'babel-loader'), options: { babelrc: false, configFile: false, presets: [path.join(modules, '@babel/preset-react')] } } },
    { test: /\.css$/, use: ['style-loader', 'css-loader'] },
  ] },
  performance: { hints: false },
}, (err, stats) => {
  clearTimeout(deadline);
  if (err || stats.hasErrors()) { console.error(err || stats.toString({ all: false, errors: true })); process.exitCode = 1; return; }
  const html = fs.readFileSync(path.join(root, 'index.html'), 'utf8').replace('<script type="module" src="/src/main.jsx"></script>', '<script defer src="/assets/app.js"></script>');
  fs.writeFileSync(path.join(root, 'dist/index.html'), html);
  console.log(stats.toString({ colors: false, modules: false, chunks: false }));
});
