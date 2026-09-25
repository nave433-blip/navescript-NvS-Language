#!/usr/bin/env ruby
root = File.expand_path("..", __dir__)
Dir.chdir(root)
env = ENV.to_h.merge("GOTOOLCHAIN" => "local", "GOPROXY" => "off", "GOSUMDB" => "off")
out = File.join(root, "bin", "nvs")
system(env, "go", "build", "-mod=mod", "-o", out, "./cmd/nvs/") or abort("go build failed")
puts `#{out} version`
system(out, "run", "examples/selfhost.ns") or abort("selfhost failed")
puts "BOOTSTRAP OK"
