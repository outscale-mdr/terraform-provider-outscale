resource "outscale_flexible_gpu" "fGPU-1" {
   model_name             =  "nvidia-k2"
   generation             =  "v3"
   subregion_name         =  "${var.region}a"
   delete_on_vm_deletion  =   false
}

