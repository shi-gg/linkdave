import { base, recommended, strict } from '@mwlica/eslint';
import tseslint from "typescript-eslint";

export default tseslint.config(
    { ignores: ['dist/**', 'eslint.config.js'] },
    base,
    recommended,
    strict
);
