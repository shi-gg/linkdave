You must follow the following guidelines

- When fixing bugs, prioritize addressing the root cause over adding error handling that simply masks the problem.
- Limit nesting depth:
    - Use functions and methods to break down complex logic.
    - Use early returns to simplify control flow.
    - Avoid if-else chains and nesting.
- Always use modern JavaScript and Go features.
- Code should be clean, readable, and maintainable.
- Prefer self-documenting code, and use comments to explain *why* a certain approach was taken, not *what* the code is doing.
- Keep code DRY (Don't Repeat Yourself).
- Keep runtime performance and memory usage in mind:
    - Write throw-away benchmarks when the performance of multiple implementation methods is unknown.
- Static globals shall always be `SCREAMING_SNAKE_CASE`.


## `client/` & `example/` & `scripts/`
- Do not use `try`-`catch` but rather `const [value, err] = await unwrap(fn())` from `client/src/utils`.
- Generally avoid using `.catch(() => null)`, as this will just silence failures.
- Do not use `any` or `unknown` type.
- Do not use nested ternary operators.
- Prefer in-house implementations over external libraries when possible.

Always check your code with: `tsgo` and `bunx eslint . --fix`.

## `server/` & `cmd/`

Always check your code with: `CGO_ENABLED=1 GOOS=linux go build -o /dev/null ./cmd/linkdave`, `go fmt ...`, `go vet ...`.


# Git

**Only perform remote/origin actions (i.e. push) with user approval**

1. Use branches for every change (`feat/this`, etc)
2. Review the changes for security, performance and bugs
3. Run the checks from the relevant section above
4. Create commit

Label every PR and issue with `agent` + one of: `bug`, `documentation`, `enhancement`.

If you can accurately express the change in just the subject line, don't include anything in the message body. Only use the body when it is providing *useful* information. Don't repeat information from the subject line in the message body.

Follow good Git style:
- Keep the subject line short
- Follow conventional commit messages: `perf:`, `refactor:`, `fix:`, `feat:`, `test:`, `ci:`, `build:`, `chore:`, `docs:`, `style:`.
- Instead of `feat: add this` use `feat: this` (no verb).
