# common.cmake - Include this in downstream projects to set up protobuf C++ paths
#
# Usage in your CMakeLists.txt:
#   include(${CMAKE_CURRENT_SOURCE_DIR}/vendor/github.com/aperturerobotics/common/common.cmake)
#
# Or for projects that have common as a direct dependency:
#   include(${CMAKE_CURRENT_SOURCE_DIR}/common.cmake)
#
# This sets up include paths so that:
#   - #include "google/protobuf/..." resolves to vendored protobuf
#   - #include "absl/..." resolves to vendored abseil-cpp
#   - #include "github.com/..." resolves to vendored Go-style paths

# Find the vendor directory
# If this file is at vendor/github.com/aperturerobotics/common/common.cmake, go up 4 levels
# If this file is at common/common.cmake (direct include), use common/vendor
get_filename_component(PROTO_CMAKE_DIR "${CMAKE_CURRENT_LIST_FILE}" DIRECTORY)
get_filename_component(PROTO_CMAKE_PARENT "${PROTO_CMAKE_DIR}" NAME)

if(PROTO_CMAKE_PARENT STREQUAL "common")
    # Direct include from common directory
    set(PROTO_VENDOR_DIR "${PROTO_CMAKE_DIR}/vendor")
else()
    # Included via vendor/github.com/aperturerobotics/common/common.cmake
    get_filename_component(PROTO_VENDOR_DIR "${PROTO_CMAKE_DIR}/../../../.." ABSOLUTE)
endif()

# Set up include paths for vendored dependencies
set(PROTO_INCLUDE_DIRS
    # For "google/protobuf/..." includes
    ${PROTO_VENDOR_DIR}/github.com/aperturerobotics/protobuf/src
    # For "utf8_validity.h" includes
    ${PROTO_VENDOR_DIR}/github.com/aperturerobotics/protobuf/third_party/utf8_range
    # For "absl/..." includes
    ${PROTO_VENDOR_DIR}/github.com/aperturerobotics/abseil-cpp
    # For "github.com/..." Go-style includes
    ${PROTO_VENDOR_DIR}
)

# Add to include directories
include_directories(${PROTO_INCLUDE_DIRS})

message(STATUS "Proto vendor dir: ${PROTO_VENDOR_DIR}")
message(STATUS "Proto include dirs: ${PROTO_INCLUDE_DIRS}")
