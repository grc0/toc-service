# Contributing

Thanks for contributing! Please follow these steps:

## Development setup
1. Install Python 3.12+ and Typst.
2. Create a virtual environment.
3. Install dependencies:
   - `pip install -r requirements.txt`
   - `pip install -r requirements-dev.txt`
4. Run the server:
   - `uvicorn app.main:app --reload --port 8000`

## Tests
- Run: `pytest`

## Pull requests
- Keep changes focused and small.
- Add or update tests where appropriate.
- Ensure `pytest` passes locally.

## Code style
- Prefer clear, explicit code.
- Avoid unrelated reformatting.
