#!/usr/bin/env python3
"""
Terraform State Migration Script for GraphQL Provider

This script helps migrate Terraform state from the old string-based variable format
to the new dynamic object format for the GraphQL provider.

Usage:
    python3 migrate_state.py <state_file> [output_file]
"""

import json
import sys
import os

def migrate_state(state_data):
    """Migrate state from string to dynamic format for variable fields."""
    resources = state_data.get('resources', [])

    for resource in resources:
        if resource.get('type') == 'graphql_mutation':
            instances = resource.get('instances', [])
            for instance in instances:
                attributes = instance.get('attributes', {})

                # Migrate mutation_variables
                if 'mutation_variables' in attributes and isinstance(attributes['mutation_variables'], str):
                    try:
                        parsed = json.loads(attributes['mutation_variables'])
                        attributes['mutation_variables'] = parsed
                        print(f"✓ Migrated mutation_variables for resource {resource.get('name', 'unknown')}")
                    except json.JSONDecodeError:
                        print(f"⚠ Warning: Could not parse mutation_variables for resource {resource.get('name', 'unknown')}")

                # Migrate read_query_variables
                if 'read_query_variables' in attributes and isinstance(attributes['read_query_variables'], str):
                    try:
                        parsed = json.loads(attributes['read_query_variables'])
                        attributes['read_query_variables'] = parsed
                        print(f"✓ Migrated read_query_variables for resource {resource.get('name', 'unknown')}")
                    except json.JSONDecodeError:
                        print(f"⚠ Warning: Could not parse read_query_variables for resource {resource.get('name', 'unknown')}")

                # Migrate delete_mutation_variables
                if 'delete_mutation_variables' in attributes and isinstance(attributes['delete_mutation_variables'], str):
                    try:
                        parsed = json.loads(attributes['delete_mutation_variables'])
                        attributes['delete_mutation_variables'] = parsed
                        print(f"✓ Migrated delete_mutation_variables for resource {resource.get('name', 'unknown')}")
                    except json.JSONDecodeError:
                        print(f"⚠ Warning: Could not parse delete_mutation_variables for resource {resource.get('name', 'unknown')}")

    return state_data

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 migrate_state.py <state_file> [output_file]")
        sys.exit(1)

    state_file = sys.argv[1]
    output_file = sys.argv[2] if len(sys.argv) > 2 else f"{state_file}.migrated"

    if not os.path.exists(state_file):
        print(f"Error: State file '{state_file}' not found")
        sys.exit(1)

    try:
        with open(state_file, 'r') as f:
            state_data = json.load(f)

        print(f"Migrating state file: {state_file}")
        migrated_state = migrate_state(state_data)

        with open(output_file, 'w') as f:
            json.dump(migrated_state, f, indent=2)

        print(f"✓ Migration completed successfully!")
        print(f"✓ Migrated state saved to: {output_file}")
        print(f"\nTo apply the migrated state:")
        print(f"  terraform state push {output_file}")

    except json.JSONDecodeError as e:
        print(f"Error: Invalid JSON in state file: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()