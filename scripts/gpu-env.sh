# gpu-env.sh is sourced by the scripts that run Passbank for tests and proofs (headless-wayland.sh, proof.sh).
# It keeps everything they start off the NVIDIA card (the user: "dont use my nvidia card to test"): EGL and GLX
# load Mesa only (glvnd would try NVIDIA's vendor library first), and the Vulkan loader sees every driver but NVIDIA's.
# The display tests of internal/gui do the same for the programs they start (step 8.6 adds that helper).

if [[ -f /usr/share/glvnd/egl_vendor.d/50_mesa.json ]]; then
  export __EGL_VENDOR_LIBRARY_FILENAMES=/usr/share/glvnd/egl_vendor.d/50_mesa.json
fi
export __GLX_VENDOR_LIBRARY_NAME=mesa
_passbank_vk=()
for _passbank_icd in /usr/share/vulkan/icd.d/*.json /etc/vulkan/icd.d/*.json; do
  [[ -f "$_passbank_icd" && "$_passbank_icd" != *nvidia* ]] && _passbank_vk+=("$_passbank_icd")
done
if ((${#_passbank_vk[@]})); then
  VK_DRIVER_FILES=$(IFS=:; echo "${_passbank_vk[*]}")
  export VK_DRIVER_FILES VK_ICD_FILENAMES="$VK_DRIVER_FILES"
fi
unset _passbank_vk _passbank_icd
