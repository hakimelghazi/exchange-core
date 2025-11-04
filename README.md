# exchange-core

Minimal matching engine prototype written in Go. The goal of this project is to explore how centralized exchanges manage order flow (placing, matching, resting) and to provide a solid code sample to discuss with recruiters.

## Project layout

- `internal/engine`: Matching engine domain types (`Order`, `Trade`) and the order book/matcher scaffolding.
- `cmd/engine`: Small CLI entrypoint that wires everything together and demonstrates how to submit orders against the matcher.

## Local development

1. Install Go 1.22 or newer.
2. Rename the module path in `go.mod` and in imports (currently `github.com/yourname/exchange-core`) to match your GitHub username once the remote repository exists.
3. Run the example engine:

   ```bash
   go run ./cmd/engine
   ```

## Next goals

- Finish the `OrderBook` implementation so bids/asks maintain proper price/size ordering.
- Flesh out the matcher logic for limit and market orders.
- Capture trades in a persistent store (memory DB first, then SQL).
- Add tests that document the expected matching behavior.

## Getting ready for GitHub

1. Initialize a clean repo in this directory: `git init`.
2. Configure your author info: `git config user.name "Your Name"` and `git config user.email "you@example.com"`.
3. Stage files: `git add .`.
4. Create your first commit: `git commit -m "chore: bootstrap exchange core project"`.
5. Create a remote repo on GitHub (e.g. `exchange-core`) and add it: `git remote add origin git@github.com:yourname/exchange-core.git`.
6. Push the main branch: `git push -u origin main`.

When you create the remote repository, remember to update the module path, rerun `go mod tidy`, and push that change as part of your initial commit or a follow-up commit.
