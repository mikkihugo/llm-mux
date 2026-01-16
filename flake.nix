{
  description = "LLM Mux - AI Gateway: Claude Pro, Copilot, Gemini subscriptions → OpenAI/Anthropic/Gemini APIs";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-25.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go toolchain
            go

            # Go development tools
            golangci-lint
            go-tools
            gotools
            gopls
            air # Live reload for development
            govulncheck
            gosec

            # Python for Grok API
            python312
            python312Packages.pip
            python312Packages.virtualenv
            python312Packages.fastapi
            python312Packages.uvicorn
            python312Packages.pydantic
            python312Packages.beautifulsoup4

            # Build tools
            gnumake
            pkg-config

            # Version control
            git

            # Utilities
            curl
            wget
            jq
          ];

          shellHook = ''
            echo "LLM Mux development environment loaded"
            echo "Go version: $(go version)"
            echo "Python version: $(python --version)"
            echo ""
            echo "Available commands:"
            echo "  Main Gateway (Go):"
            echo "    - make build           # Build the Go gateway"
            echo "    - make run             # Run the Go gateway"
            echo "    - golangci-lint run    # Run Go linter"
            echo ""
            echo "  Grok API (Python):"
            echo "    - cd grok-api && python -m venv venv && source venv/bin/activate"
            echo "    - pip install -r grok-api/requirements.txt"
            echo "    - python grok-api/api_server.py"
            echo ""
            echo "  Other:"
            echo "    - nix flake update     # Update flake.lock"
          '';
        };

        packages.default = pkgs.buildGoModule {
          pname = "llm-mux";
          version = "1.0.0";
          src = ./.;

          vendorHash = null; # Will be determined on first build
        };
      }
    );
}
