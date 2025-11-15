# Contributing to Hugo Admin

Thank you for considering contributing to Hugo Admin! We welcome contributions from the community.

## How to Contribute

### Reporting Bugs

If you find a bug, please create an issue on GitHub with:
- A clear description of the bug
- Steps to reproduce
- Expected behavior
- Actual behavior
- Your environment (OS, Python version, Hugo version)

### Suggesting Features

We'd love to hear your ideas! Please create an issue with:
- A clear description of the feature
- Use cases
- Why it would be valuable

### Pull Requests

1. **Fork the repository** and create your branch from `main`
   ```bash
   git checkout -b feature/my-new-feature
   ```

2. **Make your changes**
   - Follow the existing code style
   - Add tests if applicable
   - Update documentation as needed

3. **Test your changes**
   ```bash
   # Run tests
   pytest

   # Run with coverage
   pytest --cov=. --cov-report=html
   ```

4. **Commit your changes**
   - Use clear commit messages
   - Reference any related issues

5. **Push to your fork**
   ```bash
   git push origin feature/my-new-feature
   ```

6. **Create a Pull Request**
   - Provide a clear description of the changes
   - Link to any related issues
   - Wait for review

## Development Setup

1. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/hugo-admin.git
   cd hugo-admin
   ```

2. Create a virtual environment:
   ```bash
   python -m venv .venv
   source .venv/bin/activate  # On Windows: .venv\Scripts\activate
   ```

3. Install dependencies:
   ```bash
   pip install -r requirements-dev.txt
   ```

4. Configure for development:
   ```bash
   cp config.py config_local.py
   # Edit config_local.py with your Hugo site path
   ```

## Code Style

- Follow PEP 8 for Python code
- Use meaningful variable and function names
- Add docstrings to functions and classes
- Keep functions focused and small
- Write clear comments for complex logic

## Testing

- Write tests for new features
- Ensure all tests pass before submitting PR
- Aim for good test coverage
- Test both success and error cases

## Documentation

- Update README.md if adding new features
- Add docstrings to new functions/classes
- Update CHANGELOG.md for significant changes
- Keep documentation clear and concise

## Code of Conduct

- Be respectful and inclusive
- Welcome newcomers
- Give constructive feedback
- Focus on the code, not the person

## Questions?

Feel free to open an issue for any questions or concerns!

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
