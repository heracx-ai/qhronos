import os
import re
from collections import defaultdict

def find_go_test_files(root_dir):
    go_test_files = []
    for dirpath, _, filenames in os.walk(root_dir):
        for filename in filenames:
            if filename.endswith('_test.go'):
                go_test_files.append(os.path.join(dirpath, filename))
    return go_test_files

def extract_full_test_paths(file_path):
    with open(file_path, 'r', encoding='utf-8') as f:
        lines = f.readlines()

    # Regex patterns
    parent_pattern = re.compile(r'^func (Test\w+)\s*\(')
    subtest_pattern = re.compile(r't\.Run\(\s*"([^"]+)"')

    test_paths = set()
    stack = []
    for idx, line in enumerate(lines):
        parent_match = parent_pattern.match(line)
        if parent_match:
            # New parent test function
            stack = [(parent_match.group(1), 0)]  # (name, indent level)
            test_paths.add(parent_match.group(1))
            continue

        # Count indentation (spaces or tabs)
        indent = len(line) - len(line.lstrip(' \t'))
        subtest_match = subtest_pattern.search(line)
        if subtest_match:
            sub_name = subtest_match.group(1)
            # Pop stack if current indent is less than stack top
            while stack and indent <= stack[-1][1]:
                stack.pop()
            # Build full path
            if stack:
                full_path = '/'.join([s[0] for s in stack] + [sub_name])
            else:
                full_path = sub_name
            test_paths.add(full_path)
            stack.append((sub_name, indent))
    return test_paths

def count_all_trun_calls(root_dir):
    go_test_files = find_go_test_files(root_dir)
    trun_count = 0
    for file_path in go_test_files:
        with open(file_path, 'r', encoding='utf-8') as f:
            content = f.read()
            # Count all t.Run("...") calls
            trun_count += len(re.findall(r't\.Run\(\s*"[^"]+"', content))
    return trun_count

def main():
    root_dir = './internal'  # Change this if your tests are elsewhere
    all_trun = count_all_trun_calls(root_dir)
    print(f"Total t.Run calls found: {all_trun}")

if __name__ == '__main__':
    main() 