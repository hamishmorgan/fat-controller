// Converts internal/railway/schema.graphql → .build/railway_schema_voyager.html
// Usage: node schema-voyager.mjs <npm-prefix-dir>
// The npm prefix dir must have 'graphql' installed.

import { createRequire } from 'module';
import { readFileSync, writeFileSync } from 'fs';

const npmDir = process.argv[2];
if (!npmDir) {
  console.error('Usage: node schema-voyager.mjs <npm-prefix-dir>');
  process.exit(1);
}

const require = createRequire(import.meta.url);
const { buildSchema, introspectionFromSchema } = require(`${npmDir}/node_modules/graphql`);

const sdl = readFileSync('internal/railway/schema.graphql', 'utf8');
const introspection = introspectionFromSchema(buildSchema(sdl));
const introspectionJSON = JSON.stringify({ data: introspection });

const html = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Railway GraphQL Schema \u2014 Voyager</title>
  <style>body { margin: 0; } #voyager { height: 100vh; }</style>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/graphql-voyager@2/dist/voyager.css" />
</head>
<body>
  <div id="voyager"></div>
  <script src="https://cdn.jsdelivr.net/npm/react@18/umd/react.production.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/react-dom@18/umd/react-dom.production.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/graphql-voyager@2/dist/voyager.standalone.js"></script>
  <script>
    GraphQLVoyager.renderVoyager(
      document.getElementById('voyager'),
      { introspection: ${introspectionJSON} }
    );
  </script>
</body>
</html>`;

writeFileSync('.build/railway_schema_voyager.html', html);
console.log('Written: .build/railway_schema_voyager.html');
