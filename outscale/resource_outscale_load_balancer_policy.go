package outscale

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	oscgo "github.com/outscale/osc-sdk-go/osc"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceOutscaleAppCookieStickinessPolicy() *schema.Resource {
	return &schema.Resource{
		Create: resourceOutscaleAppCookieStickinessPolicyCreate,
		Read:   resourceOutscaleAppCookieStickinessPolicyRead,
		Delete: resourceOutscaleAppCookieStickinessPolicyDelete,

		Schema: map[string]*schema.Schema{
			"policy_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: func(v interface{}, k string) (ws []string, es []error) {
					value := v.(string)
					if !regexp.MustCompile(`^[0-9A-Za-z-]+$`).MatchString(value) {
						es = append(es, fmt.Errorf(
							"only alphanumeric characters and hyphens allowed in %q", k))
					}
					return
				},
			},
			"policy_type": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"load_balancer_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"cookie_name": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"request_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceOutscaleAppCookieStickinessPolicyCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OSCAPI

	l, lok := d.GetOk("load_balancer_name")
	p, pon := d.GetOk("policy_name")
	v, cnok := d.GetOk("cookie_name")
	pt, pot := d.GetOk("policy_type")

	if !lok && !pon && !pot {
		return fmt.Errorf("please provide the required attributes load_balancer_name, policy_name and policy_type")
	}

	vs := v.(string)
	req := oscgo.CreateLoadBalancerPolicyRequest{
		LoadBalancerName: l.(string),
		PolicyName:       p.(string),
		PolicyType:       pt.(string),
	}
	if cnok {
		req.CookieName = &vs
	}

	var err error
	var resp oscgo.CreateLoadBalancerPolicyResponse
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		resp, _, err = conn.LoadBalancerPolicyApi.
			CreateLoadBalancerPolicy(
				context.Background()).
			CreateLoadBalancerPolicyRequest(req).Execute()

		if err != nil {
			if strings.Contains(fmt.Sprint(err), "Throttling") {
				return resource.RetryableError(
					fmt.Errorf("[WARN] Error creating AppCookieStickinessPolicy, retrying: %s", err))
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("Error creating AppCookieStickinessPolicy: %s", err)
	}

	//utils.PrintToJSON(resp, "RESPONSECookie")

	reqID := ""
	if resp.ResponseContext != nil {
		reqID = *resp.ResponseContext.RequestId
	}
	d.Set("request_id", reqID)
	d.SetId(resource.UniqueId())
	d.Set("load_balancer_name", l.(string))
	d.Set("policy_name", p.(string))
	d.Set("policy_type", pt.(string))
	if cnok {
		d.Set("cookie_name", v.(string))
	}
	return resourceOutscaleAppCookieStickinessPolicyRead(d, meta)
}

func resourceOutscaleAppCookieStickinessPolicyRead(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceOutscaleAppCookieStickinessPolicyDelete(d *schema.ResourceData, meta interface{}) error {
	elbconn := meta.(*OutscaleClient).OSCAPI

	l := d.Get("load_balancer_name").(string)
	p := d.Get("policy_name").(string)

	request := oscgo.DeleteLoadBalancerPolicyRequest{
		LoadBalancerName: l,
		PolicyName:       p,
	}

	var err error
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		_, _, err = elbconn.LoadBalancerPolicyApi.
			DeleteLoadBalancerPolicy(
				context.Background()).
			DeleteLoadBalancerPolicyRequest(request).Execute()

		if err != nil {
			if strings.Contains(fmt.Sprint(err), "Throttling") {
				return resource.RetryableError(
					fmt.Errorf("[WARN] Error deleting App stickiness policy, retrying: %s", err))
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("Error deleting App stickiness policy %s: %s", d.Id(), err)
	}
	return nil
}
