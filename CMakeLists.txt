cmake_minimum_required(VERSION 3.14)
project(example_proto_test)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(Protobuf REQUIRED)

# Create symlink for this project's Go-style imports
set(VENDOR_LINK_DIR "${CMAKE_CURRENT_SOURCE_DIR}/vendor/github.com/aperturerobotics")
if(NOT EXISTS "${VENDOR_LINK_DIR}/common")
    file(CREATE_LINK "${CMAKE_CURRENT_SOURCE_DIR}" "${VENDOR_LINK_DIR}/common" SYMBOLIC)
endif()

# Include common.cmake to set up vendored protobuf/abseil include paths
include(${CMAKE_CURRENT_SOURCE_DIR}/common.cmake)

# Build a static library from the generated protobuf sources
add_library(example_proto STATIC
    example/example.pb.cc
    example/other/other.pb.cc
)

target_link_libraries(example_proto ${Protobuf_LIBRARIES})
