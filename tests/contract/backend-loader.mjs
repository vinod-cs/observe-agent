// AGENTV1 FILE START: dependency-free Node 24 read-only TypeScript contract loader.
import { registerHooks, stripTypeScriptTypes } from 'node:module';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
registerHooks({
 resolve(specifier, context, next) {
  try { return next(specifier, context); }
  catch (error) {
   if (error.code === 'ERR_MODULE_NOT_FOUND' && specifier.endsWith('.js') && (specifier.startsWith('.') || specifier.startsWith('file:'))) {
    return next(specifier.slice(0, -3) + '.ts', context);
   }
   throw error;
  }
 },
 load(url, context, next) {
  if (url.startsWith('file:') && url.endsWith('.ts')) {
   return { format: 'module', source: stripTypeScriptTypes(readFileSync(fileURLToPath(url), 'utf8')), shortCircuit: true };
  }
  return next(url, context);
 }
});
// AGENTV1 FILE END
