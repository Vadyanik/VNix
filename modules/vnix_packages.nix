{ pkgs, ... }:

{
  environment.systemPackages = with pkgs; [
    # vnix:start
      dota
    # vnix:end
  ];
}
