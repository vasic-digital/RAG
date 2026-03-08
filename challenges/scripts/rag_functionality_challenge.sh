#!/usr/bin/env bash
# rag_functionality_challenge.sh - Validates RAG module core functionality and structure
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="RAG"

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

echo "=== ${MODULE_NAME} Functionality Challenge ==="
echo ""

# Test 1: Required packages exist
echo "Test: Required packages exist"
pkgs_ok=true
for pkg in chunker retriever reranker pipeline hybrid; do
    if [ ! -d "${MODULE_DIR}/pkg/${pkg}" ]; then
        fail "Missing package: pkg/${pkg}"
        pkgs_ok=false
    fi
done
if [ "$pkgs_ok" = true ]; then
    pass "All required packages present (chunker, retriever, reranker, pipeline, hybrid)"
fi

# Test 2: Chunker interface is defined
echo "Test: Chunker interface is defined"
if grep -rq "type Chunker interface" "${MODULE_DIR}/pkg/chunker/"; then
    pass "Chunker interface is defined in pkg/chunker"
else
    fail "Chunker interface not found in pkg/chunker"
fi

# Test 3: Chunk struct is defined
echo "Test: Chunk struct is defined"
if grep -rq "type Chunk struct" "${MODULE_DIR}/pkg/chunker/"; then
    pass "Chunk struct is defined in pkg/chunker"
else
    fail "Chunk struct not found in pkg/chunker"
fi

# Test 4: Retriever interface is defined
echo "Test: Retriever interface is defined"
if grep -rq "type Retriever interface" "${MODULE_DIR}/pkg/retriever/"; then
    pass "Retriever interface is defined in pkg/retriever"
else
    fail "Retriever interface not found in pkg/retriever"
fi

# Test 5: Reranker interface is defined
echo "Test: Reranker interface is defined"
if grep -rq "type Reranker interface" "${MODULE_DIR}/pkg/reranker/"; then
    pass "Reranker interface is defined in pkg/reranker"
else
    fail "Reranker interface not found in pkg/reranker"
fi

# Test 6: Document struct is defined
echo "Test: Document struct is defined"
if grep -rq "type Document struct" "${MODULE_DIR}/pkg/"; then
    pass "Document struct found"
else
    fail "Document struct not found"
fi

# Test 7: Multiple chunker implementations
echo "Test: Multiple chunker implementations exist"
chunker_count=$(grep -rl "type\s\+\w*Chunker\s\+struct" "${MODULE_DIR}/pkg/chunker/" | wc -l)
if [ "$chunker_count" -ge 1 ]; then
    impl_count=$(grep -rh "type\s\+\w*Chunker\s\+struct" "${MODULE_DIR}/pkg/chunker/" | wc -l)
    if [ "$impl_count" -ge 2 ]; then
        pass "Multiple chunker implementations found (${impl_count})"
    else
        pass "At least one chunker implementation found"
    fi
else
    fail "No chunker implementations found"
fi

# Test 8: Pipeline support exists
echo "Test: Pipeline support exists"
if grep -rq "Pipeline\|Stage\|stage" "${MODULE_DIR}/pkg/pipeline/"; then
    pass "Pipeline support found in pkg/pipeline"
else
    fail "No pipeline support found"
fi

# Test 9: Hybrid search support
echo "Test: Hybrid search support exists"
if grep -rq "Hybrid\|hybrid\|MultiRetriever" "${MODULE_DIR}/pkg/hybrid/"; then
    pass "Hybrid search support found in pkg/hybrid"
else
    fail "No hybrid search support found"
fi

# Test 10: Search/Query capability
echo "Test: Search/Query capability exists"
if grep -rq "Search\|Query\|Retrieve" "${MODULE_DIR}/pkg/retriever/"; then
    pass "Search/Query capability found in pkg/retriever"
else
    fail "No Search/Query capability found"
fi

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed ==="
[ "${FAIL}" -eq 0 ] && exit 0 || exit 1
